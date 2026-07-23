package notifications

import (
	"context"
	"time"

	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/database"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/secrets"
	telegramapi "github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/telegram"
	"gorm.io/gorm"
)

type TelegramSender struct{ DB *gorm.DB }

func (s TelegramSender) Send(ctx context.Context, chatID int64, message string) error {
	var settings database.TelegramSetting
	if err := s.DB.WithContext(ctx).Where("id = ? AND enabled = ?", 1, true).First(&settings).Error; err != nil {
		return err
	}
	client, err := telegramapi.New(settings.APIBaseURL, secrets.TelegramToken(secrets.DefaultPath), time.Duration(settings.RequestTimeoutSeconds)*time.Second)
	if err != nil {
		return err
	}
	return client.SendMessage(ctx, chatID, message)
}
