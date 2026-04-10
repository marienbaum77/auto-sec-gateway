package bot

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/google/uuid"
	"github.com/igor/auto-sec-manager/internal/model"
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
		syncFn:   syncFn,
	}

	service.registerHandlers()
	return service, nil
}

func (s *Service) Start() {
	s.bot.Start()
}

func (s *Service) registerHandlers() {
	s.bot.Handle("/start", func(c telebot.Context) error {
		userInfo := c.Sender()
		if userInfo == nil {
			return c.Send("Could not identify Telegram user.")
		}

		var user model.User
		err := s.db.Where("telegram_id = ?", userInfo.ID).First(&user).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Send("Internal error. Please try again later.")
		}

		if errors.Is(err, gorm.ErrRecordNotFound) {
			username := userInfo.Username
			if username == "" {
				username = fmt.Sprintf("tg-%d", userInfo.ID)
			}

			user = model.User{
				TelegramID: userInfo.ID,
				Username:   username,
				UUID:       uuid.NewString(),
				Token:      uuid.NewString(),
				Active:     true,
			}

			if createErr := s.db.Create(&user).Error; createErr != nil {
				return c.Send("Failed to create subscription. Please try again later.")
			}

			if syncErr := s.syncFn(context.Background()); syncErr != nil {
				log.Printf("sync xray config failed after user create: %v", syncErr)
				return c.Send("User created, but Xray sync failed. Contact admin.")
			}
		}

		subURL := fmt.Sprintf("http://%s/sub/%s", s.publicIP, user.Token)
		return c.Send(fmt.Sprintf("Your subscription link:\n%s", subURL))
	})
}

func (s *Service) NotifyAdmin(adminID int64, text string) error {
	if adminID == 0 {
		return nil
	}
	_, err := s.bot.Send(&telebot.User{ID: adminID}, text)
	return err
}
