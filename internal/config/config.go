package config

import (
	"os"
	"strconv"
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

	// SourcesPath — файл с профилями магазинов (селекторами).
	SourcesPath string

	// AlertCooldown — пауза между алертами по одному товару.
	AlertCooldown time.Duration

	// DropPercent — минимальное снижение цены (в %) для алерта
	// «подешевело», чтобы не спамить микроколебаниями.
	DropPercent float64
}

func Load() Config {
	return Config{
		RabbitURL:        getEnv("PW_RABBIT_URL", "amqp://guest:guest@localhost:5672/"),
		SQLitePath:       getEnv("PW_SQLITE_PATH", "./data/pricepulse.db"),
		MinCheckInterval: parseDuration("PW_MIN_CHECK_INTERVAL", 30*time.Minute),
		Workers:          parseInt("PW_WORKERS", 4),
		TelegramToken:    getEnv("PW_TG_TOKEN", ""),
		TelegramChat:     getEnv("PW_TG_CHAT", ""),
		NotifierAddr:     getEnv("PW_NOTIFIER_ADDR", "localhost:50051"),
		SourcesPath:      getEnv("PW_SOURCES_PATH", "./configs/sources.yaml"),
		AlertCooldown:    parseDuration("PW_ALERT_COOLDOWN", time.Hour),
		DropPercent:      getFloat("PW_DROP_PERCENT", 3.0),
	}
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func parseDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

func parseInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return def
}

func getFloat(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}
