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

type NotifierService struct {
	notifiers []Notifier
}

type Notifiers struct {
	Telegram *Telegram `json:"telegram"`
}

func NewNotifierService(notifiers Notifiers) NotifierService {
	notifierService := NotifierService{}

	if notifiers.Telegram != nil {
		notifiers.Telegram.Init()
		notifierService.notifiers = append(notifierService.notifiers, notifiers.Telegram)
	}

	return notifierService
}

func (n NotifierService) Send(ctx context.Context, subject, message string) error {
	var eg errgroup.Group

	for _, service := range n.notifiers {
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
