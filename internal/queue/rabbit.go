package queue

import (
	"context"
	"encoding/json"
	"errors"
	"log"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Client — тонкая обёртка над amqp. RabbitMQ живёт по модели:
// Exchange (маршрутизатор) → Binding (правило) → Queue (хранитель).
// Мы используем direct exchange: сообщение с routing key X уходит
// в очередь, забинженную на ключ X. Для MVP этого достаточно.
type Client struct {
	conn *amqp.Connection
	ch   *amqp.Channel
}

// Названия очередей — константы, чтобы producer и consumer не разъехались.
const (
	TasksQueue   = "check.tasks"
	ResultsQueue = "check.results"

	// ExchangeName — общий обменник для всех сообщений pricepulse.
	ExchangeName = "pricepulse"
)

func NewClient(url string) (*Client, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, err
	}
	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, err
	}
	return &Client{conn: conn, ch: ch}, nil
}

// DeclareReady объявляет наши очереди и exchange. Идемпотентно:
// RabbitMQ разрешает повторное объявление той же конфигурации.
// durable: true — очереди и сообщения переживают рестарт брокера.
func (c *Client) DeclareReady() error {
	// Exchange: direct-тип — маршрутизация строго по совпадению
	// routing key с биндингом. durable — конфигурация переживает
	// рестарт брокера.
	if err := c.ch.ExchangeDeclare(
		ExchangeName,
		"direct", // тип: точное совпадение routing key
		true,     // durable — выживает рестарт брокера
		false,    // auto-delete — не удалять, когда уйдут потребители
		false,    // internal — false: в exchange можно публиковать извне
		false,    // no-wait — false: ждём подтверждения брокера
		nil,      // arguments — без экзотики
	); err != nil {
		return err
	}

	// Очереди. Каждая объявляется парой вызовов: QueueDeclare + QueueBind.
	for _, name := range []string{TasksQueue, ResultsQueue} {
		if _, err := c.ch.QueueDeclare(
			name,
			true,  // durable — сообщения в очереди переживают рестарт
			false, // auto-delete — не удалять при отключении потребителя
			false, // exclusive — не привязывать к одному соединению
			false, // no-wait
			nil,
		); err != nil {
			return err
		}
		// Binding: ярлык = имени очереди. Публикуешь с ключом
		// "check.tasks" — сообщение оказывается в очереди check.tasks.
		if err := c.ch.QueueBind(
			name,
			name, // routing key — для простоты равен имени очереди
			ExchangeName,
			false,
			nil,
		); err != nil {
			return err
		}
	}
	return nil
}

// PublishTask кладёт задание в очередь. JSON — да, не protobuf:
// внутри системы мы просты, а protobuf оставим там, где он оправдан —
// в контракте между сервисами (gRPC).
func (c *Client) PublishTask(ctx context.Context, v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		return err
	}
	// publish задание в очередь задач.
	return c.publish(ctx, TasksQueue, body)
}

// publish — общий механизм отправки: сериализация уже сделана,
// здесь только маршрутизация. Контекст обязателен: при зависшем
// TCP-соединении публикация с context.Background() висела бы вечно.
func (c *Client) publish(ctx context.Context, routingKey string, body []byte) error {
	return c.ch.PublishWithContext(
		ctx,
		ExchangeName, // в какой exchange
		routingKey,   // ярлык: direct exchange доставит в бинженную очередь
		false,        // mandatory — false: нет биндинга — сообщение тихо теряется
		false,        // immediate — устаревший флаг, всегда false
		amqp.Publishing{
			ContentType:  "application/json",
			Body:         body,
			DeliveryMode: amqp.Persistent, // сообщение пишется на диск — переживает рестарт брокера
		},
	)
}

func (c *Client) PublishResult(ctx context.Context, v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return c.publish(ctx, ResultsQueue, body)
}

// Close корректно закрывает канал и соединение.
func (c *Client) Close() error {
	if c.ch != nil {
		_ = c.ch.Close()
	}
	return c.conn.Close()
}

// Consume слушает очередь и зовёт handler на каждое сообщение.
// Блокируется до ctx.Done() или смерти канала — поэтому воркер
// дергает его в цикле с переподключением.
// handler выполняется в той же горутине, что и цикл чтения, —
// значит, Ack/Nack внутри безопасны (канал не трогают из других горутин).
func (c *Client) Consume(ctx context.Context, queue string, prefetch int,
	handler func(ctx context.Context, body []byte) error) error {

	ch, err := c.conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()

	// Qos/prefetch: не больше prefetch неподтверждённых сообщений
	// на воркера. Дозирование вместо «хапануть всё».
	if err := ch.Qos(prefetch, 0, false); err != nil {
		return err
	}

	msgs, err := ch.Consume(
		queue,
		"",    // consumer tag — не нужен
		false, // autoAck=false: подтверждаем ВРУЧНУЮ после обработки
		false, // exclusive
		false, // noLocal
		false, // noWait
		nil,
	)
	if err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case d, ok := <-msgs:
			if !ok {
				return errors.New("consume channel closed")
			}
			if err := handler(ctx, d.Body); err != nil {
				// Ошибка инфраструктуры (например, базы) — сообщение
				// возвращается в очередь. Повторная доставка без
				// requeue: иначе устойчивый сбой (лежащая БД) создаёт
				// мгновенный бесконечный цикл redelivery, который
				// долбит базу и логи без паузы.
				log.Printf("queue: handler failed, requeue: %v", err)
				_ = d.Nack(false, !d.Redelivered)
			} else {
				_ = d.Ack(false) // обработано: брокер удаляет сообщение
			}
		}
	}
}
