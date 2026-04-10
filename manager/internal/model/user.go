package model

// User represents a subscription client.
type User struct {
	ID         uint   `gorm:"primaryKey"`
	TelegramID int64  `gorm:"not null;uniqueIndex"`
	Username   string `gorm:"not null"`
	UUID       string `gorm:"not null;uniqueIndex"`
	Token      string `gorm:"not null;uniqueIndex"`
	Active     bool   `gorm:"not null;default:true"`
}
