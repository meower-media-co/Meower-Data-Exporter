package main

import (
	"context"
	"database/sql"
	"log"
	"os"
	"path/filepath"

	_ "github.com/glebarez/go-sqlite"
	_ "github.com/lib/pq"
	"github.com/joho/godotenv"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var ctx = context.TODO()
var rdb *redis.Client   // Redis DB (rdb)
var mdb *mongo.Database // Main DB (mdb)
var udb *sql.DB         // Uploads DB (udb)
var s3 *minio.Client    // MinIO client (s3)

func main() {
	// Load dotenv
	godotenv.Load()

	// Delete existing data exports in the output directory
	err := filepath.Walk(os.Getenv("OUTPUT_DIR"), func(path string, info os.FileInfo, err error) error {
		if path == os.Getenv("OUTPUT_DIR") || info.Name() == ".gitkeep" {
			return nil
		}
		return os.Remove(path)
	})
	if err != nil {
		log.Fatalln(err)
	}

	// Connect to Redis
	opt, err := redis.ParseURL(os.Getenv("REDIS_URI"))
	if err != nil {
		log.Fatalln(err)
	}
	rdb = redis.NewClient(opt)
	_, err = rdb.Ping(ctx).Result()
	if err != nil {
		log.Fatalln(err)
	}

	// Connect to the main MongoDB database
	serverAPI := options.ServerAPI(options.ServerAPIVersion1)
	opts := options.Client().ApplyURI(os.Getenv("MAIN_DB_URI")).SetServerAPIOptions(serverAPI)
	client, err := mongo.Connect(ctx, opts)
	if err != nil {
		log.Fatalln(err)
	}
	defer client.Disconnect(ctx)
	mdb = client.Database(os.Getenv("MAIN_DB_NAME"))

	// Test the main database connection
	var result bson.M
	if err := mdb.RunCommand(ctx, bson.D{{Key: "ping", Value: 1}}).Decode(&result); err != nil {
		log.Fatalln(err)
	}

	// Connect to the SQL uploads database
	udb, err = sql.Open(os.Getenv("UPLOADS_DB_DRIVER"), os.Getenv("UPLOADS_DB_URI"))
	if err != nil {
		log.Fatalln(err)
	}
	if err := udb.Ping(); err != nil {
		log.Fatalln(err)
	}

	// Connect to MinIO
	s3, err = minio.New(os.Getenv("MINIO_ENDPOINT"), &minio.Options{
		Creds:  credentials.NewStaticV4(os.Getenv("MINIO_ACCESS_KEY"), os.Getenv("MINIO_SECRET_KEY"), ""),
		Secure: os.Getenv("MINIO_SSL") == "1",
	})
	if err != nil {
		log.Fatalln(err)
	}
	_, err = s3.BucketExists(ctx, "data-exports")
	if err != nil {
		log.Fatalln(err)
	}

	// Tell other running instances (if there are any) to quit
	err = rdb.Publish(ctx, "data_exports", "1").Err()
	if err != nil {
		log.Fatalln(err)
	}

	// Start listening for data export requests
	pubsub := rdb.Subscribe(ctx, "data_exports")
	defer pubsub.Close()
	rdb.Publish(ctx, "data_exports", "0")
	for msg := range pubsub.Channel() {
		if msg.Payload == "0" { // Look for new data export requests
			// Get data export requests
			var dataExports []DataExport
			filter := bson.D{{Key: "status", Value: "pending"}}
			opts := options.Find().SetProjection(bson.D{{Key: "_id", Value: 1}, {Key: "user", Value: 1}})
			cur, err := mdb.Collection("data_exports").Find(ctx, filter, opts)
			if err != nil {
				log.Fatalln(err)
			}
			cur.All(ctx, &dataExports)

			// Execute all data export requests
			for _, dataExport := range dataExports {
				dataExport.execute()
			}
		} else if msg.Payload == "1" { // Quit if another instance has started running
			panic("Another instance has started running. Exiting...")
		}
	}
}
