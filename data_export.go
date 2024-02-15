package main

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"io"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/vmihailenco/msgpack/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type DataExport struct {
	Id   string `bson:"_id"`
	User string `bson:"user"`
}

func (d *DataExport) execute() {
	// Create ZIP file
	archive, err := os.Create(os.Getenv("OUTPUT_DIR") + "/" + d.Id)
	if err != nil {
		d.markAsFailed(err)
		return
	}
	defer archive.Close()

	// Create ZIP writer
	zipWriter := zip.NewWriter(archive)
	defer zipWriter.Close()

	// Run export
	err = d.runExport(zipWriter)
	if err != nil {
		d.markAsFailed(err)
		return
	}

	// Close ZIP writer and archive
	err = zipWriter.Close()
	if err != nil {
		d.markAsFailed(err)
		return
	}
	err = archive.Close()
	if err != nil {
		d.markAsFailed(err)
		return
	}

	// Upload ZIP file to MinIO
	_, err = s3.FPutObject(ctx, "data-exports", d.Id, os.Getenv("OUTPUT_DIR")+"/"+d.Id, minio.PutObjectOptions{})
	if err != nil {
		d.markAsFailed(err)
		return
	}

	// Remove ZIP file
	err = os.Remove(os.Getenv("OUTPUT_DIR") + "/" + d.Id)
	if err != nil {
		d.markAsFailed(err)
		return
	}

	// Mark as completed
	err = d.markAsCompleted()
	if err != nil {
		d.markAsFailed(err)
		return
	}
}

func (d *DataExport) markAsCompleted() error {
	// Update database item
	_, err := mdb.Collection("data_exports").UpdateOne(ctx, bson.D{{Key: "_id", Value: d.Id}}, bson.D{{Key: "$set", Value: bson.D{
		{Key: "status", Value: "completed"},
		{Key: "completed_at", Value: time.Now().Unix()},
	}}})
	if err != nil {
		return err
	}

	// Send inbox message
	marshaledEvent, err := msgpack.Marshal(map[string]string{
		"op":      "alert_user",
		"user":    d.User,
		"content": "Your data export is ready for download. You can download your data export from the [settings page](/settings) any time during the next 7 days.",
	})
	if err != nil {
		return err
	}
	err = rdb.Publish(ctx, "admin", marshaledEvent).Err()
	if err != nil {
		return err
	}

	return err
}

func (d *DataExport) markAsFailed(err error) {
	// Update database item
	_, err = mdb.Collection("data_exports").UpdateOne(ctx, bson.D{{Key: "_id", Value: d.Id}}, bson.D{{Key: "$set", Value: bson.D{
		{Key: "status", Value: "failed"},
		{Key: "error", Value: err.Error()},
		{Key: "completed_at", Value: time.Now().Unix()},
	}}})
	if err != nil {
		log.Fatalln(err)
	}

	// Send inbox message
	marshaledEvent, err := msgpack.Marshal(map[string]string{
		"op":      "alert_user",
		"user":    d.User,
		"content": "Your data export failed. Please request another data export from the [settings page](/settings). If you continue to experience issues, please contact [support@meower.org](mailto:support@meower.org).",
	})
	if err != nil {
		log.Fatalln(err)
	}
	err = rdb.Publish(ctx, "admin", marshaledEvent).Err()
	if err != nil {
		log.Fatalln(err)
	}
}

func (d *DataExport) runExport(zipWriter *zip.Writer) error {
	var err error
	var cursor *mongo.Cursor
	var fileWriter io.Writer
	var csvWriter *csv.Writer

	// Export account, settings, relationships, and netlog into user.json
	var user User
	cursor, err = mdb.Collection("usersv0").Aggregate(ctx, bson.A{
		bson.D{{Key: "$match", Value: bson.D{{Key: "_id", Value: d.User}}}},
		bson.D{
			{Key: "$lookup",
				Value: bson.D{
					{Key: "from", Value: "user_settings"},
					{Key: "localField", Value: "_id"},
					{Key: "foreignField", Value: "_id"},
					{Key: "as", Value: "settings"},
				},
			},
		},
		bson.D{
			{Key: "$unwind",
				Value: bson.D{
					{Key: "path", Value: "$settings"},
					{Key: "preserveNullAndEmptyArrays", Value: true},
				},
			},
		},
		bson.D{
			{Key: "$lookup",
				Value: bson.D{
					{Key: "from", Value: "relationships"},
					{Key: "localField", Value: "_id"},
					{Key: "foreignField", Value: "_id.from"},
					{Key: "as", Value: "relationships"},
				},
			},
		},
		bson.D{
			{Key: "$lookup",
				Value: bson.D{
					{Key: "from", Value: "netlog"},
					{Key: "localField", Value: "_id"},
					{Key: "foreignField", Value: "_id.user"},
					{Key: "as", Value: "netlogs"},
				},
			},
		},
		bson.D{
			{Key: "$project",
				Value: bson.D{
					{Key: "pswd", Value: 0},
					{Key: "tokens", Value: 0},
					{Key: "settings._id", Value: 0},
					{Key: "relationships._id.from", Value: 0},
					{Key: "netlogs._id.user", Value: 0},
				},
			},
		},
	}, options.Aggregate())
	if err != nil {
		return err
	}
	defer cursor.Close(ctx)
	for cursor.Next(ctx) {
		err = cursor.Decode(&user)
		if err != nil {
			return err
		}
	}
	marshaledUser, err := json.MarshalIndent(user, "", "\t")
	if err != nil {
		return err
	}
	fileWriter, err = zipWriter.Create("user.json")
	if err != nil {
		return err
	}
	_, err = io.Copy(fileWriter, bytes.NewReader(marshaledUser))
	if err != nil {
		return err
	}
	zipWriter.Flush()

	// Export reports
	var reports []Report
	cursor, err = mdb.Collection("reports").Find(ctx, bson.D{{Key: "reports.user", Value: d.User}}, options.Find())
	if err != nil {
		return err
	}
	err = cursor.All(ctx, &reports)
	if err != nil {
		return err
	}
	fileWriter, err = zipWriter.Create("safety/reports.csv")
	if err != nil {
		return err
	}
	csvWriter = csv.NewWriter(fileWriter)
	csvWriter.Write([]string{
		"id",
		"type",
		"content_id",
		"status",
		"ip",
		"reason",
		"comment",
		"time",
	})
	for _, report := range reports {
		for _, reporter := range report.Reporters {
			if reporter.User == d.User {
				csvWriter.Write([]string{
					report.Id,
					report.Type,
					report.ContentId,
					report.Status,
					reporter.Ip,
					reporter.Reason,
					reporter.Comment,
					strconv.FormatInt(reporter.Time, 10),
				})
				break
			}
		}
	}
	csvWriter.Flush()
	zipWriter.Flush()

	// Export chats
	var chats []Chat
	cursor, err = mdb.Collection("chats").Find(ctx, bson.D{{Key: "members", Value: d.User}}, options.Find())
	if err != nil {
		return err
	}
	err = cursor.All(ctx, &chats)
	if err != nil {
		return err
	}
	for _, chat := range chats {
		marshaledChat, err := json.MarshalIndent(chat, "", "\t")
		if err != nil {
			return err
		}
		fileWriter, err = zipWriter.Create("chats/" + chat.Id + ".json")
		if err != nil {
			return err
		}
		_, err = io.Copy(fileWriter, bytes.NewReader(marshaledChat))
		if err != nil {
			return err
		}
	}
	zipWriter.Flush()

	// Get post origins
	postOrigins, err := mdb.Collection("posts").Distinct(ctx, "post_origin", bson.D{{Key: "u", Value: d.User}}, options.Distinct())
	if err != nil {
		return err
	}

	// Export posts
	for _, postOrigin := range postOrigins {
		var posts []Post
		cursor, err = mdb.Collection("posts").Aggregate(ctx, bson.A{
			bson.D{{Key: "$match", Value: bson.D{
				{Key: "u", Value: d.User},
				{Key: "post_origin", Value: postOrigin},
			}}},
			bson.D{
				{Key: "$lookup",
					Value: bson.D{
						{Key: "from", Value: "post_revisions"},
						{Key: "localField", Value: "_id"},
						{Key: "foreignField", Value: "post_id"},
						{Key: "as", Value: "revisions"},
					},
				},
			},
		}, options.Aggregate())
		if err != nil {
			return err
		}
		err = cursor.All(ctx, &posts)
		if err != nil {
			return err
		}

		fileWriter, err = zipWriter.Create("posts/" + postOrigin.(string) + ".csv")
		if err != nil {
			return err
		}
		csvWriter = csv.NewWriter(fileWriter)
		csvWriter.Write([]string{
			"id",
			"content",
			"unfiltered_content",
			"timestamp",
			"revisions",
			"edited_at",
			"deleted",
			"mod_deleted",
			"deleted_at",
		})
		for _, post := range posts {
			marshaledRevisions, err := json.Marshal(post.Revisions)
			if err != nil {
				return err
			}

			var unfilteredContent string
			if post.UnfilteredContent != nil {
				unfilteredContent = *post.UnfilteredContent
			}

			var editedAt string
			if post.EditedAt != nil {
				editedAt = strconv.FormatInt(*post.EditedAt, 10)
			}

			var deletedAt string
			if post.DeletedAt != nil {
				deletedAt = strconv.FormatInt(*post.DeletedAt, 10)
			}

			csvWriter.Write([]string{
				post.Id,
				post.Content,
				unfilteredContent,
				strconv.FormatInt(post.Timestamp.Epoch, 10),
				string(marshaledRevisions),
				editedAt,
				strconv.FormatBool(post.Deleted),
				strconv.FormatBool(post.ModDeleted),
				deletedAt,
			})
		}
		csvWriter.Flush()
		zipWriter.Flush()
	}

	// Export icon uploads
	fileWriter, err = zipWriter.Create("uploads/icons.csv")
	if err != nil {
		return err
	}
	csvWriter = csv.NewWriter(fileWriter)
	csvWriter.Write([]string{
		"id",
		"hash",
		"mime",
		"size",
		"width",
		"height",
		"uploaded_at",
		"used_by",
	})
	rows, err := udb.Query("SELECT * FROM icons WHERE uploader = ?", d.User)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var iconUpload IconUpload
		err = rows.Scan(
			&iconUpload.Id,
			&iconUpload.Hash,
			&iconUpload.Mime,
			&iconUpload.Size,
			&iconUpload.Width,
			&iconUpload.Height,
			&iconUpload.UploadedBy,
			&iconUpload.UploadedAt,
			&iconUpload.UsedBy,
		)
		if err != nil {
			return err
		}
		csvWriter.Write([]string{
			iconUpload.Id,
			iconUpload.Hash,
			iconUpload.Mime,
			strconv.FormatInt(iconUpload.Size, 10),
			strconv.Itoa(iconUpload.Width),
			strconv.Itoa(iconUpload.Height),
			strconv.FormatInt(iconUpload.UploadedAt, 10),
			iconUpload.UsedBy,
		})
	}
	csvWriter.Flush()

	// Export attachment uploads
	fileWriter, err = zipWriter.Create("uploads/attachments.csv")
	if err != nil {
		return err
	}
	csvWriter = csv.NewWriter(fileWriter)
	csvWriter.Write([]string{
		"id",
		"hash",
		"mime",
		"filename",
		"size",
		"width",
		"height",
		"uploaded_at",
		"used_by",
	})
	rows, err = udb.Query("SELECT * FROM attachments WHERE uploader = ?", d.User)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var attachmentUpload AttachmentUpload
		err = rows.Scan(
			&attachmentUpload.Id,
			&attachmentUpload.Hash,
			&attachmentUpload.Mime,
			&attachmentUpload.Filename,
			&attachmentUpload.Size,
			&attachmentUpload.Width,
			&attachmentUpload.Height,
			&attachmentUpload.UploadedBy,
			&attachmentUpload.UploadedAt,
			&attachmentUpload.UsedBy,
		)
		if err != nil {
			return err
		}
		csvWriter.Write([]string{
			attachmentUpload.Id,
			attachmentUpload.Hash,
			attachmentUpload.Mime,
			attachmentUpload.Filename,
			strconv.FormatInt(attachmentUpload.Size, 10),
			strconv.Itoa(attachmentUpload.Width),
			strconv.Itoa(attachmentUpload.Height),
			strconv.FormatInt(attachmentUpload.UploadedAt, 10),
			attachmentUpload.UsedBy,
		})
	}
	csvWriter.Flush()

	return nil
}
