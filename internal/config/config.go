package config

import (
	"os"
	"time"
)

// Config — все настройки из переменных окружения.
// Соглашение 12-factor: ничего не хардкодим, всё через env.
type Config struct {
	// Адрес RabbitMQ в формате amqp://guest:guest@localhost:5672/
	RabbitURL string

	// SQLite лежит в volume, чтобы данные переживали перезапуск.
	SQLitePath string

	// MinCheckInterval — минимальная пауза между проверками одного товара.
	// Вежливость к чужим сайтам важнее нетерпеливости.
	MinCheckInterval time.Duration

	// Workers — число одновременных проверок в воркере.
	Workers int

	// Telegram bot token и chat id для notifier'а.
	TelegramToken string
	TelegramChat  string

	// gRPC-адрес notifier'а для воркера, listen-адрес для самого notifier.
	NotifierAddr string
}

func Load() Config {
	return Config{
		RabbitURL:  getEnv("PW_RABBIT_URL", "amqp://guest:guest@localhost:5672/"),
		SQLitePath: getEnv("PW_SQLITE_PATH", "./data/pricepulse.db"),
		// TODO-1: parseDuration — читает env как секунды
		// и даёт дефолт 30 минут. Подсказка: strconv.Atoi, затем
		// time.Duration(n) * time.Second.
		MinCheckInterval: parseDuration("PW_MIN_CHECK_INTERVAL", 30*time.Minute),
		// TODO-2: parseInt — Workers из PW_WORKERS, дефолт 4.
		Workers:       parseInt("PW_WORKERS", 4),
		TelegramToken: getEnv("PW_TG_TOKEN", ""),
		TelegramChat:  getEnv("PW_TG_CHAT", ""),
		NotifierAddr:  getEnv("PW_NOTIFIER_ADDR", "localhost:50051"),
	}
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// TODO-3: parseDuration и parseInt.
// parseDuration(key string, def time.Duration) time.Duration
// parseInt(key string, def int) int
