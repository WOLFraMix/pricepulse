package main

import (
	"context"
	"log"
	"os"
	"os/signal"

	"github.com/WOLFraMix/pricepulse/internal/alert"
	"github.com/WOLFraMix/pricepulse/internal/config"
	"github.com/WOLFraMix/pricepulse/internal/queue"
	"github.com/WOLFraMix/pricepulse/internal/scraper"
	"github.com/WOLFraMix/pricepulse/internal/store"
	"github.com/WOLFraMix/pricepulse/internal/worker"
)

func main() {
	cfg := config.Load()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	st, err := store.New(cfg.SQLitePath)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer func() { _ = st.Close() }()

	qc, err := queue.NewClient(cfg.RabbitURL)
	if err != nil {
		log.Fatalf("queue: %v", err)
	}
	defer func() { _ = qc.Close() }()

	if err := qc.DeclareReady(); err != nil {
		log.Fatalf("declare: %v", err)
	}

	scrCfg, err := scraper.LoadProfiles(cfg.SourcesPath)
	if err != nil {
		log.Fatalf("sources: %v", err)
	}

	// Ленивый клиент: подключится при первом алерте, поэтому порядок
	// запуска сервисов не важен.
	alerts, err := alert.New(cfg.NotifierAddr)
	if err != nil {
		log.Fatalf("alert client: %v", err)
	}
	defer func() { _ = alerts.Close() }()

	log.Printf("worker: running with %d workers", cfg.Workers)
	worker.New(cfg, st, qc, scraper.New(scrCfg), alerts).Run(ctx)
	log.Printf("worker: stopped")
}
