package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Telegram — самый простой способ отправить сообщение: официальный
// Bot API, один HTTP POST. Для MVP этого достаточно с запасом.
type Telegram struct {
	token string
	chat  string
	http  *http.Client
}

func NewTelegram(token, chat string) *Telegram {
	return &Telegram{
		token: token,
		chat:  chat,
		http:  &http.Client{Timeout: 65 * time.Second},
	}
}

func (t *Telegram) Send(ctx context.Context, text string) error {
	payload, err := json.Marshal(map[string]string{
		"chat_id": t.chat,
		"text":    text,
	})
	if err != nil {
		return err
	}
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram api: http %d", resp.StatusCode)
	}
	return nil
}
