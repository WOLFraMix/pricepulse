package queue

import (
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
	// TODO-4: по документации amqp091-go сделать три вещи
	// одним вызовом каждой: ExchangeDeclare (direct, "pricepulse",
	// durable), QueueDeclare (TasksQueue и ResultsQueue, durable),
	// QueueBind (обе очереди на exchange с routing key = имени очереди).
	// Подсказка: c.ch.ExchangeDeclare, c.ch.QueueDeclare, c.ch.QueueBind.
	return nil
}

// PublishTask кладёт задание в очередь. JSON — да, не protobuf:
// внутри системы мы просты, а protobuf оставим там, где он оправдан —
// в контракте между сервисами (gRPC).
func (c *Client) PublishTask(v any) error {
	// TODO-5: json.Marshal → c.ch.PublishWithContext
	// (exchange "pricepulse", routing key TasksQueue). И не забыть
	// mandatory: false — если очередь пропадёт, задание не вернётся бултыхом.
	return nil
}
