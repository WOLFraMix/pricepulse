package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/WOLFraMix/pricepulse/internal/api"
	"github.com/WOLFraMix/pricepulse/internal/config"
	"github.com/WOLFraMix/pricepulse/internal/model"
	"github.com/WOLFraMix/pricepulse/internal/queue"
	"github.com/WOLFraMix/pricepulse/internal/store"
)

func main() {
	cfg := config.Load()

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

	// Планировщик: раз в MinCheckInterval кладёт задачу для каждого
	// активного товара. MVP-решение — жить в процессе api; вынести
	// в отдельный cmd/scheduler можно в любой момент, он зависит
	// только от store и queue.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		ticker := time.NewTicker(cfg.MinCheckInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				products, err := st.Products(true)
				if err != nil {
					log.Printf("scheduler: %v", err)
					continue
				}
				for _, p := range products {
					task := model.CheckTask{
						ProductID:   p.ID,
						URL:         p.URL,
						Source:      p.Source,
						TargetPrice: p.TargetPrice,
						DueAt:       time.Now(),
					}
					// Таймаут на публикацию: при зависшем AMQP-соединении
					// тик планировщика не должен виснуть навечно.
					pubCtx, pubCancel := context.WithTimeout(ctx, 5*time.Second)
					if err := qc.PublishTask(pubCtx, task); err != nil {
						log.Printf("scheduler: publish %d: %v", p.ID, err)
					}
					pubCancel()
				}
				if len(products) > 0 {
					log.Printf("scheduler: published %d tasks", len(products))
				}
			}
		}
	}()

	addr := os.Getenv("PW_API_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	log.Printf("api: running on %s", addr)
	if err := api.New(st).Run(addr); err != nil {
		log.Fatalf("api: %v", err)
	}
}
