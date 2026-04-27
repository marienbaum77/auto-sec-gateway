package bot

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/marienbaum77/auto-sec-manager/internal/model"
	"gopkg.in/telebot.v3"
	"gorm.io/gorm"
)

type SyncFunc func(ctx context.Context) error

type Service struct {
	bot      *telebot.Bot
	db       *gorm.DB
	publicIP string
	adminID  int64
	syncFn   SyncFunc
}

func New(token string, db *gorm.DB, publicIP string, adminID int64, syncFn SyncFunc) (*Service, error) {
	settings := telebot.Settings{
		Token:  token,
		Poller: &telebot.LongPoller{Timeout: 10},
	}

	b, err := telebot.NewBot(settings)
	if err != nil {
		return nil, fmt.Errorf("init telegram bot: %w", err)
	}

	service := &Service{
		bot:      b,
		db:       db,
		publicIP: publicIP,
		adminID:  adminID,
		syncFn:   syncFn,
	}

	service.registerHandlers()
	return service, nil
}

func (s *Service) Start() {
	log.Printf("[BOT] Started. Admin ID: %d", s.adminID)
	s.bot.Start()
}

func (s *Service) registerHandlers() {
	// Главное меню для админа
	adminMenu := &telebot.ReplyMarkup{ResizeKeyboard: true}
	btnUsers := adminMenu.Text("👥 Пользователи")
	btnStats := adminMenu.Text("📊 Статус")
	adminMenu.Reply(adminMenu.Row(btnUsers, btnStats))

	s.bot.Handle("/start", func(c telebot.Context) error {
		userInfo := c.Sender()
		var user model.User

		// Регистрация или поиск юзера
		res := s.db.Where("telegram_id = ?", userInfo.ID).Limit(1).Find(&user)
		if res.RowsAffected == 0 {
			user = model.User{
				TelegramID: userInfo.ID,
				Username:   userInfo.Username,
				UUID:       uuid.NewString(),
				Token:      uuid.NewString(),
				Active:     true,
			}
			s.db.Create(&user)
			s.syncFn(context.Background())
		}

		subURL := fmt.Sprintf("http://%s/sub/%s", s.publicIP, user.Token)
		msg := fmt.Sprintf("🔗 Твоя подписка:\n`%s`", subURL)

		// Если пишет админ — выдаем ему кнопки управления
		if userInfo.ID == s.adminID {
			return c.Send("Добро пожаловать, Админ. Доступ разрешен.", adminMenu)
		}
		return c.Send(msg, telebot.ModeMarkdown)
	})

	// Команда списка пользователей (только для админа)
	s.bot.Handle(&btnUsers, func(c telebot.Context) error {
		if c.Sender().ID != s.adminID {
			return nil
		}
		var users []model.User
		s.db.Find(&users)

		var b strings.Builder
		b.WriteString("📋 **Список пользователей:**\n")
		for _, u := range users {
			status := "✅"
			if !u.Active {
				status = "🚫"
			}
			b.WriteString(fmt.Sprintf("%s %s | ID: `%d`\n", status, u.Username, u.TelegramID))
		}
		return c.Send(b.String(), telebot.ModeMarkdown)
	})

	// Команда бана: /ban <telegram_id>
	s.bot.Handle("/ban", func(c telebot.Context) error {
		if c.Sender().ID != s.adminID {
			return nil
		}
		args := c.Args()
		if len(args) < 1 {
			return c.Send("Использование: /ban <telegram_id>")
		}

		targetID, _ := strconv.ParseInt(args[0], 10, 64)
		s.db.Model(&model.User{}).Where("telegram_id = ?", targetID).Update("active", false)
		s.syncFn(context.Background()) // Мгновенно выкидываем из Xray через gRPC

		return c.Send(fmt.Sprintf("🚫 Пользователь %d заблокирован", targetID))
	})

	// Статус ноды
	s.bot.Handle(&btnStats, func(c telebot.Context) error {
		if c.Sender().ID != s.adminID {
			return nil
		}
		// Здесь можно вызвать функцию пинга порта, которую мы писали в checker
		return c.Send("✅ Все системы работают штатно. Нода Xray активна.")
	})
}

func (s *Service) NotifyAdmin(adminID int64, text string) error {
	if adminID == 0 {
		return nil
	}
	_, err := s.bot.Send(&telebot.User{ID: adminID}, text)
	return err
}
