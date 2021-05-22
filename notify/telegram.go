package notify

import (
	"context"
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/pkg/errors"
)

const defaultParseMode = tgbotapi.ModeHTML

type Telegram struct {
	client    *tgbotapi.BotAPI
	ApiToken  string  `json:"apiToken"`
	Receivers []int64 `json:"receivers"`
}

func (t *Telegram) Init() {
	client, err := tgbotapi.NewBotAPI(t.ApiToken)
	if err != nil {
		log.Fatal(err)
	}
	t.client = client
}

func (t Telegram) Send(ctx context.Context, subject, message string) error {
	fullMessage := subject + "\n" + message

	msg := tgbotapi.NewMessage(0, fullMessage)
	msg.ParseMode = defaultParseMode

	for _, chatID := range t.Receivers {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			msg.ChatID = chatID
			_, err := t.client.Send(msg)
			if err != nil {
				return errors.Wrapf(err, "failed to send message to Telegram chat '%d'", chatID)
			}
		}
	}

	return nil
}
