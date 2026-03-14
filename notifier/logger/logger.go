package logger

import (
	"context"
	"log"
)

type Logger struct {
}

func New() *Logger {
	return &Logger{}
}

func (l *Logger) Notify(_ context.Context, subject, message string) error {
	log.Printf("Notification: %s - %s", subject, message)
	return nil
}
