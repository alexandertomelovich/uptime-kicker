package notifier

import (
	"context"
)

// ParseModeHTML — значение ParseMode для сообщений, в которых допускается
// HTML-разметка. Все динамические значения (имена, URL, тексты ошибок) в таком
// сообщении обязаны быть экранированы через EscapeHTML.
const ParseModeHTML = "HTML"

type Message struct {
	ChatID    int64
	Text      string
	ParseMode string
}

type Sender interface {
	Send(ctx context.Context, message Message) error
}
