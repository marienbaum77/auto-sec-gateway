package main

import (
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/marienbaum77/auto-sec-gateway/internal/api"
	"github.com/marienbaum77/auto-sec-gateway/internal/bot"
	"github.com/marienbaum77/auto-sec-gateway/internal/checker"
	"github.com/marienbaum77/auto-sec-gateway/internal/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func main() {
	dsn := getEnv("DATABASE_URL", "")
	if dsn == "" {
		log.Fatal("[Fatal] Переменная окружения DATABASE_URL не задана")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("[Fatal] Ошибка подключения к базе данных: %v", err)
	}

	if err := db.AutoMigrate(&model.User{}, &model.Administrator{}, &model.Node{}, &model.Metric{}); err != nil {
		log.Fatalf("[Fatal] Ошибка миграции таблиц БД: %v", err)
	}

	publicIP := getEnv("PUBLIC_IP", "127.0.0.1")
	tgToken := os.Getenv("TELEGRAM_TOKEN")
	if tgToken == "" {
		log.Fatal("[Fatal] Переменная окружения TELEGRAM_TOKEN не задана")
	}

	monitor := checker.NewMonitor(db)
	tgBot, err := bot.New(tgToken, db, publicIP, monitor.Check)
	if err != nil {
		log.Fatalf("[Fatal] Ошибка инициализации Telegram-бота: %v", err)
	}
	monitor.RegisterAlertHandler(tgBot.NotifyAllAdmins)
	go tgBot.Start()
	r := gin.Default()
	apiServer := api.NewServer(db, publicIP)
	apiServer.Register(r)

	log.Println("[API] API-сервер запущен на порту :8080")
	if err := r.Run(":8080"); err != nil {
		log.Fatalf("[Fatal] Ошибка запуска HTTP-сервера: %v", err)
	}
}
