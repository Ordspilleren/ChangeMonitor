package main

import (
	"github.com/nikoksr/notify"
	"github.com/nikoksr/notify/service/mailgun"
	"github.com/nikoksr/notify/service/telegram"
)

type Notifier interface {
	Init() notify.Notifier
}

type NotifierMap map[string]notify.Notifier

type Notifiers struct {
	Mailgun  *Mailgun  `json:"mailgun"`
	Telegram *Telegram `json:"telegram"`
}

type Mailgun struct {
	Domain        string   `json:"domain"`
	ApiKey        string   `json:"apiKey"`
	SenderAddress string   `json:"senderAddress"`
	WithEurope    bool     `json:"withEurope"`
	Receivers     []string `json:"receivers"`
}

type Telegram struct {
	ApiToken  string  `json:"apiToken"`
	Receivers []int64 `json:"receivers"`
}

func (n *Notifiers) InitNotifiers() NotifierMap {
	notifiers := make(NotifierMap)

	if n.Mailgun != nil {
		notifiers["mailgun"] = n.Mailgun.Init()
	}
	if n.Telegram != nil {
		notifiers["telegram"] = n.Telegram.Init()
	}

	return notifiers
}

func (m Mailgun) Init() notify.Notifier {
	notifier := mailgun.New(m.Domain, m.ApiKey, m.SenderAddress, mailgun.WithEurope())
	notifier.AddReceivers(m.Receivers...)
	return notifier
}

func (m Telegram) Init() notify.Notifier {
	notifier, _ := telegram.New(m.ApiToken)
	notifier.AddReceivers(m.Receivers...)
	return notifier
}
