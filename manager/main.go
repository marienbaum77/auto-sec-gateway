package main

import (
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/igor/auto-sec-manager/internal/checker"
	"github.com/igor/auto-sec-manager/internal/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var (
	db         *gorm.DB
	node       = &model.Node{ID: "yandex-node-1", Port: 8443, Status: "Init", IsAlive: false}
	publicAddr string
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

	if err := db.AutoMigrate(&model.User{}); err != nil {
		log.Fatal("Failed to migrate database:", err)
	}

	var count int64
	if err := db.Model(&model.User{}).Count(&count).Error; err != nil {
		log.Fatal("Failed to count users:", err)
	}

	if count == 0 {
		if err := db.Create(&model.User{
			Username: "admin",
			UUID:     "99c0b456-bd47-46d9-81a8-fc2920a7548a",
			Token:    "my-secret-token",
			Active:   true,
		}).Error; err != nil {
			log.Fatal("Failed to create test user:", err)
		}
		log.Println("Test user created: admin")
	}
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func monitorNode() {
	target := getEnv("XRAY_ADDR", "127.0.0.1:8443")
	for {
		latency, err := checker.CheckPort(target, 5*time.Second)
		if err != nil {
			node.IsAlive = false
			node.Status = "Blocked"
		} else {
			node.IsAlive = true
			node.Status = "Healthy"
			_ = latency // можно логировать
		}
		time.Sleep(15 * time.Second)
	}
}

func handleSub(c *gin.Context) {
	token := c.Param("token")
	var user model.User

	if err := db.Where("token = ? AND active = ?", token, true).First(&user).Error; err != nil {
		c.String(404, "Invalid or inactive subscription")
		return
	}

	if !node.IsAlive {
		c.String(200, "")
		return
	}

	vlessURL := fmt.Sprintf(
		"vless://%s@%s:%d?security=reality&sni=dl.google.com&fp=chrome&pbk=rgKtm9CVStTQUs7MfGmIdj6BAKaSwpjEReVrxNLspU8&sid=abcdef12&type=tcp&flow=xtls-rprx-vision#Sovereign-%s",
		user.UUID, publicAddr, node.Port, user.Username,
	)

	encoded := base64.StdEncoding.EncodeToString([]byte(vlessURL + "\n"))
	c.String(200, encoded)
}

func main() {
	publicAddr = getEnv("PUBLIC_IP", "158.160.40.150")
	
	initDB()
	go monitorNode()

	r := gin.Default()
	r.GET("/sub/:token", handleSub)
	r.Run(":8080")
}