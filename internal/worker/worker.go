package worker

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/WOLFraMix/pricepulse/internal/alert"
	"github.com/WOLFraMix/pricepulse/internal/config"
	"github.com/WOLFraMix/pricepulse/internal/model"
	"github.com/WOLFraMix/pricepulse/internal/queue"
	"github.com/WOLFraMix/pricepulse/internal/scraper"
	"github.com/WOLFraMix/pricepulse/internal/store"
)

// Worker — пул консюмеров. Каждый «воркер» — своя горутина со своим
// AMQP-каналом: каналы не потокобезопасны, поэтому параллелизм = N каналов.
type Worker struct {
	cfg    config.Config
	st     *store.Store
	qc     *queue.Client
	scr    *scraper.Scraper
	alerts *alert.Client

	// alerted — cooldown-память «товар → время последнего алерта».
	// sync.Map удобен для кейса «мало ключей, много чтений из разных
	// горутин». Честная оговорка: это память процесса — после рестарта
	// воркера забудет; в проде такое состояние переносят в базу.
	alerted sync.Map
}

func New(cfg config.Config, st *store.Store, qc *queue.Client, scr *scraper.Scraper, alerts *alert.Client) *Worker {
	return &Worker{cfg: cfg, st: st, qc: qc, scr: scr, alerts: alerts}
}

// Run запускает пул и блокируется до отмены контекста.
func (w *Worker) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for i := 0; i < w.cfg.Workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			w.loop(ctx, id)
		}(i)
	}
	wg.Wait()
}

func (w *Worker) loop(ctx context.Context, id int) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		err := w.qc.Consume(ctx, queue.TasksQueue, 1, w.handle)
		if ctx.Err() != nil {
			return
		}
		log.Printf("worker %d: consume dropped: %v; reconnect in 2s", id, err)
		time.Sleep(2 * time.Second)
	}
}

func (w *Worker) handle(ctx context.Context, body []byte) error {
	var task model.CheckTask
	if err := json.Unmarshal(body, &task); err != nil {
		log.Printf("poison task dropped: %v (body: %s)", err, body)
		return nil
	}

	price, title, currency, err := w.scr.Check(ctx, task.URL, task.Source)
	if err != nil {
		log.Printf("check product %d: %v", task.ProductID, err)
		return nil
	}

	if title != "" {
		if err := w.st.SetTitle(task.ProductID, title); err != nil {
			return err
		}
	}
	if err := w.st.AddPricePoint(model.PricePoint{
		ProductID: task.ProductID,
		Price:     price,
		Currency:  currency,
		CheckedAt: time.Now(),
	}); err != nil {
		return err
	}

	log.Printf("product %d: %q = %.2f %s", task.ProductID, title, price, currency)

	if price <= task.TargetPrice {
		w.notify(ctx, task, title, currency, price)
	}
	return nil
}

// notify — с защитой от спама: не чаще AlertCooldown на товар.
// Ошибка доставки не роняет обработку: точка цены уже записана,
// алерт дотянется в следующий тик.
func (w *Worker) notify(ctx context.Context, task model.CheckTask, title, currency string, price float64) {
	if last, ok := w.alerted.Load(task.ProductID); ok {
		if time.Since(last.(time.Time)) < w.cfg.AlertCooldown {
			return // недавно уже кричали — молчим
		}
	}
	if err := w.alerts.Alert(ctx, task, title, currency, price); err != nil {
		log.Printf("notify product %d: %v", task.ProductID, err)
		return
	}
	w.alerted.Store(task.ProductID, time.Now())
	log.Printf(">>> ALERT sent for product %d (%.2f <= %.2f)", task.ProductID, price, task.TargetPrice)
}
