package model

import "gorm.io/gorm"

// User — класс пользователя системы (для UML)
type User struct {
	gorm.Model
	Username string `gorm:"uniqueIndex"` // Имя пользователя
	UUID     string // Его личный ключ в Xray
	Token    string `gorm:"uniqueIndex"`  // Его токен для ссылки подписки
	Active   bool   `gorm:"default:true"` // Статус доступа
}
