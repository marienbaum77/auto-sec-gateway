package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
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
	db            *gorm.DB
	publicIP      string
	xrayPort      int
	xrayPublicKey string
	shortID       string
	nodeAlive     atomic.Bool
)

const gistFileName = "subscription.txt"

// UpdateGist writes Base64-encoded newline-separated VLESS URLs of all active users to a GitHub Gist.
// Requires GITHUB_TOKEN and GIST_ID. Patches file subscription.txt in the gist.
func UpdateGist() {
	token := os.Getenv("GITHUB_TOKEN")
	gistID := os.Getenv("GIST_ID")
	if token == "" || gistID == "" {
		return
	}

	var users []model.User
	if err := db.Where("active = ?", true).Order("id").Find(&users).Error; err != nil {
		log.Printf("UpdateGist: load users: %v", err)
		return
	}

	lines := make([]string, 0, len(users))
	for _, u := range users {
		line := fmt.Sprintf(
			"vless://%s@%s:%d?security=reality&sni=dl.google.com&fp=chrome&pbk=%s&sid=%s&type=tcp&flow=xtls-rprx-vision#Sovereign-%s",
			u.UUID, publicIP, xrayPort, xrayPublicKey, shortID, u.Username,
		)
		lines = append(lines, line)
	}
	combined := strings.Join(lines, "\n")
	encoded := base64.StdEncoding.EncodeToString([]byte(combined))

	body := map[string]any{
		"files": map[string]any{
			gistFileName: map[string]string{"content": encoded},
		},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		log.Printf("UpdateGist: marshal request: %v", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	url := fmt.Sprintf("https://api.github.com/gists/%s", gistID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, bytes.NewReader(payload))
	if err != nil {
		log.Printf("UpdateGist: build request: %v", err)
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("UpdateGist: request: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("UpdateGist: GitHub API returned %s", resp.Status)
	}
}

func syncXrayAndUpdateGist(ctx context.Context) error {
	err := k8sync.SyncXrayConfig(ctx, db)
	UpdateGist()
	return err
}

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

	xrayPublicKey = getEnv("XRAY_PUBLIC_KEY", "")
	shortID = getEnv("SHORT_ID", "abcdef12")

	tgToken := os.Getenv("TELEGRAM_TOKEN")
	adminID := int64(getEnvInt("ADMIN_ID", 0))

	var tgBot *bot.Service
	var err error
	if tgToken != "" {
		tgBot, err = bot.New(
			tgToken,
			db,
			publicIP,
			syncXrayAndUpdateGist,
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

	if err := syncXrayAndUpdateGist(context.Background()); err != nil {
		log.Printf("initial Xray sync failed: %v", err)
	}

	go monitorNode(adminID, tgBot)

	r := gin.Default()
	apiServer := api.NewServer(db, publicIP, xrayPublicKey, shortID, xrayPort, &nodeAlive)
	apiServer.Register(r)

	r.Run(":8080")
}
