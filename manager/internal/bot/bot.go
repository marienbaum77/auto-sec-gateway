package bot

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/marienbaum77/auto-sec-gateway/internal/model"
	"gopkg.in/telebot.v3"
	"gorm.io/gorm"
)

type SyncFunc func(ctx context.Context) error

type Service struct {
	bot      *telebot.Bot
	db       *gorm.DB
	publicIP string
	syncFn   SyncFunc
}

func New(token string, db *gorm.DB, publicIP string, syncFn SyncFunc) (*Service, error) {
	b, err := telebot.NewBot(telebot.Settings{Token: token, Poller: &telebot.LongPoller{Timeout: 10}})
	if err != nil {
		return nil, err
	}

	s := &Service{bot: b, db: db, publicIP: publicIP, syncFn: syncFn}
	s.registerHandlers()
	return s, nil
}

func (s *Service) Start() {
	s.bot.Start()
}

// isAdmin проверяет наличие записи в таблице administrators для указанного telegramID через SQL JOIN
func (s *Service) isAdmin(telegramID int64) bool {
	var count int64
	err := s.db.Table("administrators").
		Joins("JOIN users ON users.id = administrators.user_id").
		Where("users.telegram_id = ?", telegramID).
		Count(&count).Error

	return err == nil && count > 0
}

func (s *Service) registerHandlers() {
	// Обработчик для обычных пользователей (Регистрация при первом входе)
	s.bot.Handle("/start", func(c telebot.Context) error {
		userInfo := c.Sender()
		var user model.User

		if err := s.db.Where("telegram_id = ?", userInfo.ID).First(&user).Error; err != nil {
			safeUsername := userInfo.Username
			if safeUsername == "" {
				safeUsername = userInfo.FirstName
				if safeUsername == "" {
					safeUsername = fmt.Sprintf("user_%d", userInfo.ID)
				}
			}

			user = model.User{
				TelegramID: userInfo.ID,
				Username:   safeUsername,
				UUID:       uuid.NewString(),
				Token:      uuid.NewString(),
				Active:     true,
			}

			if err := s.db.Create(&user).Error; err != nil {
				log.Printf("[ERROR] Не удалось создать пользователя: %v", err)
				return c.Send("❌ Ошибка регистрации.")
			}
		}

		baseURL := os.Getenv("SUBSCRIPTION_BASE_URL")
		if baseURL == "" {
			baseURL = fmt.Sprintf("http://%s", s.publicIP)
		}
		subURL := fmt.Sprintf("%s/sub/%s", strings.TrimRight(baseURL, "/"), user.Token)

		return c.Send(subURL, telebot.ModeMarkdown)
	})

	// 1. CREATE: Создание пользователя вручную (например, без Telegram)
	s.bot.Handle("/create", func(c telebot.Context) error {
		if !s.isAdmin(c.Sender().ID) {
			return nil
		}
		args := c.Args()
		if len(args) < 2 {
			return c.Send("⚠️ Формат: `/create <username> <telegram_id>`\nЕсли у пользователя нет Telegram, укажите уникальное отрицательное число.", telebot.ModeMarkdown)
		}
		username := args[0]
		tgID, err := strconv.ParseInt(args[1], 10, 64)
		if err != nil {
			return c.Send("❌ Некорректный Telegram ID.")
		}

		user := model.User{
			TelegramID: tgID,
			Username:   username,
			UUID:       uuid.NewString(),
			Token:      uuid.NewString(),
			Active:     true,
		}
		if err := s.db.Create(&user).Error; err != nil {
			return c.Send(fmt.Sprintf("❌ Ошибка создания пользователя: %v", err))
		}

		baseURL := os.Getenv("SUBSCRIPTION_BASE_URL")
		if baseURL == "" {
			baseURL = fmt.Sprintf("http://%s", s.publicIP)
		}
		subURL := fmt.Sprintf("%s/sub/%s", strings.TrimRight(baseURL, "/"), user.Token)

		response := fmt.Sprintf("✅ *Пользователь создан!*\n\n• *ID*: `%d`\n• *Имя*: %s\n• *UUID*: `%s`\n• *Ссылка*: %s", tgID, username, user.UUID, subURL)
		return c.Send(response, telebot.ModeMarkdown)
	})

	// 2. READ: Просмотр списка всех пользователей системы
	s.bot.Handle("/users", func(c telebot.Context) error {
		if !s.isAdmin(c.Sender().ID) {
			return nil
		}
		var users []model.User
		if err := s.db.Find(&users).Error; err != nil {
			return c.Send("❌ Ошибка получения списка.")
		}
		if len(users) == 0 {
			return c.Send("Список пользователей пуст.")
		}

		var b strings.Builder
		b.WriteString("📋 *Список пользователей шлюза:*\n\n")
		for _, u := range users {
			status := "🟢 Активен"
			if !u.Active {
				status = "🔴 Заблокирован"
			}
			b.WriteString(fmt.Sprintf("• *ID*: `%d` | *Имя*: %s | %s\n", u.TelegramID, u.Username, status))
		}
		return c.Send(b.String(), telebot.ModeMarkdown)
	})

	// 3. READ: Просмотр детальной информации по конкретному Telegram ID
	s.bot.Handle("/info", func(c telebot.Context) error {
		if !s.isAdmin(c.Sender().ID) {
			return nil
		}
		args := c.Args()
		if len(args) == 0 {
			return c.Send("⚠️ Формат: `/info <telegram_id>`", telebot.ModeMarkdown)
		}
		tgID, _ := strconv.ParseInt(args[0], 10, 64)

		var user model.User
		if err := s.db.Where("telegram_id = ?", tgID).First(&user).Error; err != nil {
			return c.Send("❌ Пользователь не найден.")
		}

		status := "🟢 Активен"
		if !user.Active {
			status = "🔴 Заблокирован"
		}

		baseURL := os.Getenv("SUBSCRIPTION_BASE_URL")
		if baseURL == "" {
			baseURL = fmt.Sprintf("http://%s", s.publicIP)
		}
		subURL := fmt.Sprintf("%s/sub/%s", strings.TrimRight(baseURL, "/"), user.Token)

		response := fmt.Sprintf("👤 *Информация о пользователе:*\n\n• *ID*: `%d`\n• *Имя*: %s\n• *Статус*: %s\n• *UUID*: `%s`\n• *Ссылка*: %s", user.TelegramID, user.Username, status, user.UUID, subURL)
		return c.Send(response, telebot.ModeMarkdown)
	})

	// 4. UPDATE: Изменение статуса активности пользователя (активация/деактивация)
	s.bot.Handle("/update_status", func(c telebot.Context) error {
		if !s.isAdmin(c.Sender().ID) {
			return nil
		}
		args := c.Args()
		if len(args) < 2 {
			return c.Send("⚠️ Формат: `/update_status <telegram_id> <true/false>`", telebot.ModeMarkdown)
		}
		tgID, _ := strconv.ParseInt(args[0], 10, 64)
		activeVal := args[1] == "true"

		var user model.User
		if err := s.db.Where("telegram_id = ?", tgID).First(&user).Error; err != nil {
			return c.Send("❌ Пользователь не найден.")
		}

		user.Active = activeVal
		if err := s.db.Save(&user).Error; err != nil {
			return c.Send("❌ Ошибка сохранения статуса.")
		}

		status := "🟢 Активирован"
		if !activeVal {
			status = "🔴 Деактивирован"
		}
		return c.Send(fmt.Sprintf("✅ Пользователь %s (%d): %s", user.Username, user.TelegramID, status))
	})

	// 5. DELETE/UPDATE: Быстрая блокировка пользователя (бан)
	s.bot.Handle("/ban", func(c telebot.Context) error {
		if !s.isAdmin(c.Sender().ID) {
			return nil
		}
		args := c.Args()
		if len(args) > 0 {
			targetID, _ := strconv.ParseInt(args[0], 10, 64)
			s.db.Model(&model.User{}).Where("telegram_id = ?", targetID).Update("active", false)
			return c.Send("🚫 Доступ заблокирован")
		}
		return nil
	})
}

// NotifyAdmin отправляет текстовое уведомление администратору напрямую по его Telegram ID
func (s *Service) NotifyAdmin(adminID int64, text string) error {
	recipient := telebot.ChatID(adminID)

	_, err := s.bot.Send(recipient, text, telebot.ModeMarkdown)
	if err != nil {
		log.Printf("[ERROR] Не удалось отправить уведомление администратору %d: %v", adminID, err)
		return err
	}

	log.Printf("[Bot] Уведомление успешно отправлено администратору %d", adminID)
	return nil
}

// NotifyAllAdmins выбирает всех администраторов из БД и рассылает им сообщение
func (s *Service) NotifyAllAdmins(text string) {
	var adminIDs []int64

	// Извлекаем все telegram_id из таблицы пользователей, связанных с администраторами
	err := s.db.Table("administrators").
		Joins("JOIN users ON users.id = administrators.user_id").
		Pluck("users.telegram_id", &adminIDs).Error

	if err != nil {
		log.Printf("[ERROR] Не удалось получить ID администраторов из БД: %v", err)
		return
	}

	for _, id := range adminIDs {
		_ = s.NotifyAdmin(id, text)
	}
}
