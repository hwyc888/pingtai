package model

type Identity struct {
	ID          string `json:"id"`
	Provider    string `json:"provider"`
	Subject     string `json:"subject"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	LocalUserID int64  `json:"local_user_id,omitempty"`
}
