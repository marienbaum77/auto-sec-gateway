package checker

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/marienbaum77/auto-sec-gateway/internal/model"
	"gorm.io/gorm"
)

type HealthMonitor struct {
	db      *gorm.DB
	onAlert func(text string)
}

func NewMonitor(db *gorm.DB) *HealthMonitor {
	return &HealthMonitor{db: db}
}

func (hm *HealthMonitor) RegisterAlertHandler(fn func(text string)) {
	hm.onAlert = fn
}

func (hm *HealthMonitor) Check(ctx context.Context) error {
	var nodes []model.Node
	if err := hm.db.Find(&nodes).Error; err != nil {
		log.Printf("[Monitor] Ошибка получения списка нод из БД: %v", err)
		return err
	}

	for _, node := range nodes {
		oldIsAlive := node.IsAlive
		oldStatus := node.Status

		hostAddress := fmt.Sprintf("%s:%d", node.ID, node.Port)
		latency, err := CheckHTTPS(hostAddress, 5*time.Second)

		metric := model.Metric{
			NodeID:    node.ID,
			Timestamp: time.Now(),
			Latency:   latency,
			Success:   err == nil,
		}

		if err != nil {
			metric.ErrorType = err.Error()
			node.Status = "Blocked"
			node.IsAlive = false
		} else {
			node.Status = "Healthy"
			node.IsAlive = true
		}
		node.LastPing = time.Now()

		if hm.onAlert != nil {
			if oldIsAlive && !node.IsAlive {
				msg := fmt.Sprintf("⚠️ *Сбой сетевого шлюза!*\n\n• *Узел*: `%s` (порт %d)\n• *Статус*: `Недоступен`\n• *Ошибка*: `%s`", node.ID, node.Port, metric.ErrorType)
				hm.onAlert(msg)
			}
			if !oldIsAlive && node.IsAlive && oldStatus != "Unknown" && oldStatus != "" {
				msg := fmt.Sprintf("🟢 *Сетевой шлюз восстановлен!*\n\n• *Узел*: `%s` (порт %d)\n• *Статус*: `Доступен`\n• *Задержка*: `%v`", node.ID, node.Port, latency)
				hm.onAlert(msg)
			}
		}

		if err := hm.db.Create(&metric).Error; err != nil {
			log.Printf("[Monitor] Не удалось записать метрику для %s: %v", node.ID, err)
		}

		if err := hm.db.Save(&node).Error; err != nil {
			log.Printf("[Monitor] Не удалось обновить статус узла %s: %v", node.ID, err)
		}
	}

	return nil
}
