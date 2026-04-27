package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/marienbaum77/auto-sec-manager/internal/api"
	"github.com/marienbaum77/auto-sec-manager/internal/bot"
	"github.com/marienbaum77/auto-sec-manager/internal/checker"
	"github.com/marienbaum77/auto-sec-manager/internal/model"
	"github.com/marienbaum77/auto-sec-manager/internal/xray"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var (
	db            *gorm.DB
	publicIP      string
	xrayPort      int
	xrayPublicKey string
	shortID       string
	nodeAlive     atomic.Bool
)

// syncXray — сердце системы. Выгружает пользователей из БД в память Xray по gRPC.
func syncXray(ctx context.Context) error {
	log.Println("[SYNC] Начало синхронизации состояния БД -> Xray-core...")

	var users []model.User
	if err := db.Where("active = ?", true).Find(&users).Error; err != nil {
		return fmt.Errorf("ошибка чтения БД: %w", err)
	}

	xClient := xray.NewClient("127.0.0.1:10085")

	successCount := 0
	for _, u := range users {
		err := xClient.AddUser(ctx, u.Username, u.UUID, "xtls-rprx-vision")
		if err != nil {
			log.Printf("[SYNC WARNING] Не удалось добавить %s: %v", u.Username, err)
			continue
		}
		successCount++
	}

	log.Printf("[SYNC] Успешно загружено %d из %d пользователей", successCount, len(users))
	return nil
}

func initDB() {
	dsn := getEnv("DATABASE_URL", "")
	var err error

	for i := 0; i < 5; i++ {
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if err == nil {
			break
		}
		log.Printf("Waiting for database... attempt %d", i+1)
		time.Sleep(5 * time.Second)
	}

	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
}

func migrateUsers() {
	if err := db.AutoMigrate(&model.User{}); err != nil {
		log.Fatal("Failed to migrate users table:", err)
	}
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

// monitorNode реализует логику самолечения (Self-healing)
func monitorNode(adminID int64, tgBot *bot.Service) {
	target := getEnv("XRAY_ADDR", "127.0.0.1:8443")
	notifiedDown := false
	wasDown := false

	for {
		_, err := checker.CheckPort(target, 5*time.Second)
		if err != nil {
			nodeAlive.Store(false)
			wasDown = true
			if !notifiedDown && tgBot != nil && adminID != 0 {
				tgBot.NotifyAdmin(adminID, "🚨 Xray node is down! Connections are lost.")
				notifiedDown = true
			}
		} else {
			nodeAlive.Store(true)

			// Если узел только что восстановился — возвращаем пользователей в память
			if wasDown {
				log.Println("[RECOVERY] Узел ожил. Восстанавливаю состояние пользователей...")
				if err := syncXray(context.Background()); err != nil {
					log.Printf("[RECOVERY ERROR] Ошибка: %v", err)
				} else {
					if tgBot != nil && adminID != 0 {
						tgBot.NotifyAdmin(adminID, "✅ Xray recovered. State synchronized.")
					}
					wasDown = false
				}
			}
			notifiedDown = false
		}
		time.Sleep(15 * time.Second)
	}
}

func getEnvInt(key string, fallback int) int {
	raw := getEnv(key, "")
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

func main() {
	publicIP = getEnv("PUBLIC_IP", "")
	xrayPort = getEnvInt("XRAY_PORT", 8443)
	xrayPublicKey = getEnv("XRAY_PUBLIC_KEY", "")
	shortID = getEnv("SHORT_ID", "abcdef12")

	initDB()
	migrateUsers()

	tgToken := os.Getenv("TELEGRAM_TOKEN")
	adminID := int64(getEnvInt("ADMIN_ID", 0))

	var tgBot *bot.Service
	var err error
	if tgToken != "" {
		tgBot, err = bot.New(tgToken, db, publicIP, adminID, syncXray)
		if err != nil {
			log.Fatalf("failed to initialize telegram bot: %v", err)
		}
		go tgBot.Start()
	}

	// Первичная загрузка при старте
	if err := syncXray(context.Background()); err != nil {
		log.Printf("Initial sync failed: %v", err)
	}

	go monitorNode(adminID, tgBot)

	r := gin.Default()
	apiServer := api.NewServer(db, publicIP, xrayPublicKey, shortID, xrayPort, &nodeAlive)
	apiServer.Register(r)

	r.Run(":8080")
}
