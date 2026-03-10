package notifier

import (
	"context"
	"log"
)

// Notifier sends alerts to the user.
type Notifier interface {
	// Notify sends a notification for a matched listing.
	Notify(ctx context.Context, subject string, message string) error
}

type Notifiers []Notifier

type NotifierService struct {
	notifiers Notifiers
}

func NewNotifierService(notifiers Notifiers) *NotifierService {
	return &NotifierService{notifiers: notifiers}
}

func (s *NotifierService) Notify(ctx context.Context, subject, message string) error {
	for _, notifier := range s.notifiers {
		if err := notifier.Notify(ctx, subject, message); err != nil {
			// Log the error but continue with other notifiers.
			log.Printf("notifier error: %v", err)
		}
	}
	return nil
}
