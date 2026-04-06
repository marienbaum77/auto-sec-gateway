package main

import (
	"encoding/base64"
	"fmt"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/igor/auto-sec-manager/internal/checker"
	"github.com/igor/auto-sec-manager/internal/model"
)

// Глобальные переменные состояния (для UML это ассоциация "Manager -> Node")
var (
	node = &model.Node{
		ID:      "yandex-node-1",
		Port:    8443,
		Status:  "Initializing",
		IsAlive: false,
	}
	publicAddr string
)

// Константы для Reality (в реальном проекте выносятся в конфиг/БД)
const (
	UUID       = "99c0b456-bd47-46d9-81a8-fc2920a7548a"
	PUBLIC_KEY = "rgKtm9CVStTQUs7MfGmIdj6BAKaSwpjEReVrxNLspU8"
	SNI        = "dl.google.com"
	SHORT_ID   = "abcdef12"
)

// getEnv читает переменную окружения или возвращает значение по умолчанию
func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

// monitorNode — фоновый процесс (Goroutine), реализующий логику Health Check
func monitorNode() {
	// Адрес, который мы реально пингуем (локальный или удаленный)
	checkTarget := getEnv("XRAY_ADDR", "127.0.0.1:8443")
	
	fmt.Printf("[Monitor] Starting heuristic analysis on: %s\n", checkTarget)

	for {
		// Вызываем метод из нашего внутреннего пакета checker
		latency, err := checker.CheckPort(checkTarget, 5*time.Second)
		
		if err != nil {
			fmt.Printf("[%s] ALERT: Node деградировал или заблокирован! Error: %v\n", 
				time.Now().Format("15:04:05"), err)
			node.IsAlive = false
			node.Status = "Blocked"
		} else {
			fmt.Printf("[%s] Node OK. Latency: %v\n", 
				time.Now().Format("15:04:05"), latency)
			node.IsAlive = true
			node.Status = "Healthy"
		}

		// Пауза между проверками
		time.Sleep(15 * time.Second)
	}
}

// handleSub — обработчик API для выдачи подписки (Base64 VLESS link)
func handleSub(c *gin.Context) {
	token := c.Param("token")
	fmt.Printf("[API] Subscription request received with token: %s\n", token)

	// Логика: если узел мертв, мы не выдаем его в подписке (защита пользователя)
	if !node.IsAlive {
		c.String(200, "") // Пустая подписка, если всё упало
		return
	}

	// Формируем стандартную строку VLESS с параметрами Reality
	// Формат: vless://uuid@ip:port?param=value#name
	vlessURL := fmt.Sprintf(
		"vless://%s@%s:%d?security=reality&sni=%s&fp=chrome&pbk=%s&sid=%s&type=tcp&flow=xtls-rprx-vision#Sovereign-Node",
		UUID,
		publicAddr,
		node.Port,
		SNI,
		PUBLIC_KEY,
		SHORT_ID,
	)

	// v2ray клиенты ожидают список ссылок, закодированный в Base64
	encoded := base64.StdEncoding.EncodeToString([]byte(vlessURL + "\n"))
	
	c.String(200, encoded)
}

func main() {
	// 1. Инициализация параметров из окружения
	publicAddr = getEnv("PUBLIC_IP", "158.160.40.150")
	
	// 2. Запуск "Мозга" в фоновом потоке
	go monitorNode()

	// 3. Настройка HTTP-сервера (API Gateway)
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	// Эндпоинт для подписки
	r.GET("/sub/:token", handleSub)

	// Простая проверка здоровья самого менеджера
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "Manager is running",
			"node":   node.Status,
		})
	})

	fmt.Printf("[Server] Subscription API is live on :8080\n")
	fmt.Printf("[Info] Local sub link: http://127.0.0.1:8080/sub/my-secret-token\n")
	
	// Запуск сервера
	if err := r.Run(":8080"); err != nil {
		fmt.Errorf("Failed to start server: %v", err)
	}
}