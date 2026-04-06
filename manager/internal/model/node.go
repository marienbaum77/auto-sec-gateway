package model

import "time"

// Node представляет твой прокси-узел
type Node struct {
	ID       string
	Address  string
	Port     int
	Status   string // "Healthy", "Degraded", "Blocked"
	IsAlive  bool   // Флаг живости узла
	LastPing time.Time
}

// Metric — результат одного замера
type Metric struct {
	Timestamp time.Time
	Latency   time.Duration
	Success   bool
	ErrorType string // Например, "TLS_Handshake_Timeout"
}
