package models

// Token is an application password for a given user.
type Token struct {
	ID        uint   `gorm:"primaryKey" json:"id"`
	UserEmail string `gorm:"size:255;not null;index:idx_tokens_user_email" json:"user_email"`
	Password  string `gorm:"size:255;not null" json:"-"`
	IP        string `gorm:"size:4096" json:"ip"`
}
