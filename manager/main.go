package main

import (
	"context"
	"log"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/igor/auto-sec-manager/internal/api"
	"github.com/igor/auto-sec-manager/internal/bot"
	"github.com/igor/auto-sec-manager/internal/checker"
	k8sync "github.com/igor/auto-sec-manager/internal/k8s"
	"github.com/igor/auto-sec-manager/internal/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"k8s.io/client-go/rest"
)

var (
	db        *gorm.DB
	publicIP  string
	xrayPort  int
	nodeAlive atomic.Bool
)

func initDB() {
	dsn := getEnv("DATABASE_URL", "host=localhost user=manager password=mypassword dbname=manager port=5432 sslmode=disable")
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

func monitorNode(adminID int64, tgBot *bot.Service) {
	target := getEnv("XRAY_ADDR", "127.0.0.1:8443")
	notifiedDown := false

	for {
		_, err := checker.CheckPort(target, 5*time.Second)
		if err != nil {
			nodeAlive.Store(false)
			if !notifiedDown && tgBot != nil && adminID != 0 {
				if notifyErr := tgBot.NotifyAdmin(adminID, "Xray node is unavailable: "+err.Error()); notifyErr != nil {
					log.Printf("failed to notify admin: %v", notifyErr)
				} else {
					notifiedDown = true
				}
			}
		} else {
			nodeAlive.Store(true)
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
	publicIP = getEnv("PUBLIC_IP", "158.160.40.150")
	xrayPort = getEnvInt("XRAY_PORT", 8443)

	initDB()
	migrateUsers()

	xrayPublicKey := getEnv("XRAY_PUBLIC_KEY", "")
	xrayPrivateKey := getEnv("XRAY_PRIVATE_KEY", "")

	tgToken := os.Getenv("TELEGRAM_TOKEN")
	adminID := int64(getEnvInt("ADMIN_ID", 0))

	var tgBot *bot.Service
	var err error
	if tgToken != "" {
		tgBot, err = bot.New(
			tgToken,
			db,
			publicIP,
			func(ctx context.Context) error {
				return k8sync.SyncXrayConfig(ctx, db)
			},
		)
		if err != nil {
			log.Fatalf("failed to initialize telegram bot: %v", err)
		}
		go tgBot.Start()
	} else {
		log.Println("TELEGRAM_TOKEN is empty, bot is disabled")
	}

	if _, err := rest.InClusterConfig(); err != nil {
		log.Printf("kubernetes in-cluster config unavailable: %v", err)
	}

	if err := k8sync.SyncXrayConfig(context.Background(), db); err != nil {
		log.Printf("initial Xray sync failed: %v", err)
	}

	go monitorNode(adminID, tgBot)

	r := gin.Default()
	apiServer := api.NewServer(db, publicIP, xrayPublicKey, xrayPrivateKey, xrayPort, &nodeAlive)
	apiServer.Register(r)

	r.Run(":8080")
}
