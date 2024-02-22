package main

type User struct {
	Id          string `bson:"uuid" json:"id"`
	Username    string `bson:"_id" json:"username"`
	PfpData     int64  `bson:"pfp_data" json:"pfp_data"`
	Quote       string `bson:"quote" json:"quote"`
	Flags       int64  `bson:"flags" json:"flags"`
	Permissions int64  `bson:"permissions" json:"permissions"`
	Ban         struct {
		State        string `bson:"state" json:"state"`
		Restrictions int64  `bson:"restrictions" json:"restrictions"`
		Expires      int64  `bson:"expires" json:"expires"`
		Reason       string `bson:"reason" json:"reason"`
	} `bson:"ban" json:"ban"`
	Created       int64                  `bson:"created" json:"created"`
	LastSeen      *int64                 `bson:"last_seen,omitempty" json:"last_seen"`
	DeleteAfter   *int64                 `bson:"delete_after,omitempty" json:"delete_after"`
	Settings      map[string]interface{} `bson:"settings" json:"settings"`
	Relationships []Relationship         `bson:"relationships" json:"relationships"`
	Netlogs       []Netlog               `bson:"netlogs" json:"netlogs"`
}

type Relationship struct {
	Id struct {
		To string `bson:"to" json:"to"`
	} `bson:"_id" json:"id"`
	State     int   `bson:"state" json:"state"`
	UpdatedAt int64 `bson:"updated_at" json:"updated_at"`
}

type Netlog struct {
	Id struct {
		Ip string `bson:"ip" json:"ip"`
	} `bson:"_id" json:"id"`
	LastUsed int64 `bson:"last_used" json:"last_used"`
}

type Report struct {
	Id        string `bson:"_id"`
	Type      string `bson:"type"`
	ContentId string `bson:"content_id"`
	Status    string `bson:"status"`
	Reporters []struct {
		User    string `bson:"user"`
		Ip      string `bson:"ip"`
		Reason  string `bson:"reason"`
		Comment string `bson:"comment"`
		Time    int64  `bson:"time"`
	} `bson:"reports"`
}

type Chat struct {
	Id         string   `bson:"_id" json:"id"`
	Type       int      `bson:"type" json:"type"`
	Nickname   string   `bson:"nickname" json:"nickname"`
	Owner      string   `bson:"owner" json:"owner"`
	Members    []string `bson:"members" json:"members"`
	Created    int64    `bson:"created" json:"created"`
	LastActive int64    `bson:"last_active" json:"last_active"`
	Deleted    bool     `bson:"deleted" json:"deleted"`
}

type Post struct {
	Id                string  `bson:"_id"`
	Content           string  `bson:"p"`
	UnfilteredContent *string `bson:"unfiltered_p,omitempty"`
	Timestamp         struct {
		Epoch int64 `bson:"e"`
	} `bson:"t"`
	Revisions []struct {
		Id         string `bson:"_id" json:"id"`
		OldContent string `bson:"old_content" json:"old_content"`
		NewContent string `bson:"new_content" json:"new_content"`
		Time       int64  `bson:"time" json:"time"`
	} `bson:"revisions"`
	EditedAt   *int64 `bson:"edited_at,omitempty"`
	Deleted    bool   `bson:"isDeleted"`
	ModDeleted bool   `bson:"mod_deleted"`
	DeletedAt  *int64 `bson:"deleted_at,omitempty"`
}

type IconUpload struct {
	Id         string
	Hash       string
	Mime       string
	Size       int64
	Width      int
	Height     int
	Uploader string
	UploadedAt int64
	UsedBy     string
}

type AttachmentUpload struct {
	Id         string
	Hash       string
	Mime       string
	Filename   string
	Size       int64
	Width      int
	Height     int
	Uploader string
	UploadedAt int64
	UsedBy     string
}
