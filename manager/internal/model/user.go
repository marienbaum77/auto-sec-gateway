package model

type User struct {
	ID         uint   `gorm:"primaryKey"`
	TelegramID int64  `gorm:"not null;uniqueIndex"`
	Username   string `gorm:"not null"`
	UUID       string `gorm:"not null;uniqueIndex"`
	Token      string `gorm:"not null;uniqueIndex"`
	Active     bool   `gorm:"not null;default:true"`
}
type Administrator struct {
	UserID uint `gorm:"primaryKey"` 
	User   User `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE;"`
}
