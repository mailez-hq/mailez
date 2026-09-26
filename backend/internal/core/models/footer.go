package models

type OrgFooter struct {
	Base
	ID       uint   `gorm:"primaryKey" json:"id"`
	Domain   string `gorm:"size:80;not null;uniqueIndex" json:"domain"`
	BodyHTML string `gorm:"type:text" json:"body_html"`
	BodyText string `gorm:"type:text" json:"body_text"`
	Enabled  bool   `gorm:"not null;default:false" json:"enabled"`
}
