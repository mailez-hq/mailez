package models

import "time"

// BrandingConfig holds the login-page branding customized by the deploying
// organization. It is a single row (id = 1); empty fields fall back to the
// built-in Mailez brand in the frontend.
type BrandingConfig struct {
	ID uint `gorm:"primaryKey" json:"id"`
	// Title is the brand name shown in the top-left corner (default "Mailez").
	Title string `gorm:"size:128" json:"title"`
	// Subtitle appears next to the logo, e.g. "电子邮件系统" / "Email System".
	Subtitle string `gorm:"size:255" json:"subtitle"`
	// Tagline is the headline on the left branding panel.
	Tagline string `gorm:"size:255" json:"tagline"`
	// Feature1-3 are the bullet points on the left branding panel.
	Feature1 string `gorm:"size:255" json:"feature1"`
	Feature2 string `gorm:"size:255" json:"feature2"`
	Feature3 string `gorm:"size:255" json:"feature3"`
	// LogoURL is the top-left logo image (absolute URL or a served path).
	LogoURL string `gorm:"size:1024" json:"logo_url"`
	// HeroURL is the background image of the left branding panel.
	HeroURL string `gorm:"size:1024" json:"hero_url"`
	// Copyright is the footer text, e.g. "Copyright © its.tju.edu.cn, All Rights Reserved".
	Copyright string `gorm:"size:255" json:"copyright"`
	// UpdatedAt reflects the last save (admin console).
	UpdatedAt time.Time `json:"updated_at"`
}
