package notify

import (
	"context"

	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
)

type Notifier interface {
	Init()
	Send(context.Context, string, string) error
}

type NotifierList []Notifier

type NotifierMap map[string]Notifier

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

func (n *Notifiers) InitNotifiers() NotifierMap {
	notifiers := make(NotifierMap)

	if n.Telegram != nil {
		n.Telegram.Init()
		notifiers["telegram"] = n.Telegram
	}

	return notifiers
}

func (n NotifierList) Send(ctx context.Context, subject, message string) error {
	var eg errgroup.Group

	for _, service := range n {
		if service != nil {
			s := service
			eg.Go(func() error {
				return s.Send(ctx, subject, message)
			})
		}
	}

	err := eg.Wait()
	if err != nil {
		err = errors.Wrap(err, err.Error())
	}

	return err
}
