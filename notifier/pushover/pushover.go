package pushover

import (
	"context"
	"fmt"

	"github.com/gregdel/pushover"
)

// Notifier sends Pushover notifications.
type Notifier struct {
	app       *pushover.Pushover
	recipient *pushover.Recipient
}

// New creates a Notifier with the given API token and user key.
func New(apiToken, userKey string) *Notifier {
	return &Notifier{
		app:       pushover.New(apiToken),
		recipient: pushover.NewRecipient(userKey),
	}
}

// Notify sends a Pushover notification with the given subject and message.
func (n *Notifier) Notify(_ context.Context, subject, message string) error {
	msg := pushover.NewMessageWithTitle(message, subject)
	_, err := n.app.SendMessage(msg, n.recipient)
	if err != nil {
		return fmt.Errorf("pushover send: %w", err)
	}
	return nil
}
