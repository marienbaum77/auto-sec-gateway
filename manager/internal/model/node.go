package model

import "time"

type Node struct {
	ID       string `gorm:"primaryKey"`
	Port     int    `gorm:"not null"`
	Status   string `gorm:"not null;default:'Unknown'"`
	IsAlive  bool   `gorm:"not null;default:false"`
	LastPing time.Time
}

type Metric struct {
	ID        uint          `gorm:"primaryKey"`
	NodeID    string        `gorm:"not null;index"`
	Node      Node          `gorm:"foreignKey:NodeID;references:ID;constraint:OnDelete:CASCADE;"`
	Timestamp time.Time     `gorm:"not null"`
	Latency   time.Duration `gorm:"not null"`
	Success   bool          `gorm:"not null"`
	ErrorType string
}
