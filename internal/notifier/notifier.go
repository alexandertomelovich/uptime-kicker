package notifier

import (
	"context"
)

type Message struct {
	ChatID int64
	Text string
	ParseMode string
}

type Sender interface {
	Send(ctx context.Context, message Message) error
}

