package model

import "time"

type User struct {
	ID        int64      "json:\"id\""
	Username  string     "json:\"username\""
	Password  string     "json:\"-\""
	UserType  int        "json:\"usertype\""
	Status    int        "json:\"status\""
	DelTag    int        "json:\"deltag\""
	OpenID    string     "json:\"openid\""
	BindTime  *time.Time "json:\"bind_time\""
	CreatedAt *time.Time "json:\"created_at\""
	UpdatedAt *time.Time "json:\"updated_at\""
}
