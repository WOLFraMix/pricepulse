package alert

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/WOLFraMix/pricepulse/internal/model"
	"github.com/WOLFraMix/pricepulse/internal/pb"
)

// Client — обёртка над сгенерированным стабом. Работе в воркере
// нужен не весь grpc-мир, а один понятный метод Alert.
type Client struct {
	conn *grpc.ClientConn
	api  pb.NotifierClient
}

// New создаёт клиента. grpc.NewClient (в новых версиях grpc-go
// он вместо устаревшего grpc.Dial) ленив: реальное соединение
// откроется при первом вызове, поэтому notifier может стартовать позже.
func New(addr string) (*Client, error) {
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return &Client{conn: conn, api: pb.NewNotifierClient(conn)}, nil
}

func (c *Client) Alert(ctx context.Context, task model.CheckTask, title, currency string, price float64) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	resp, err := c.api.SendAlert(ctx, &pb.AlertRequest{
		ProductId:   task.ProductID,
		Title:       title,
		Url:         task.URL,
		Price:       price,
		Currency:    currency,
		TargetPrice: task.TargetPrice,
	})
	if err != nil {
		return err // сеть/сервер недоступны — ошибка транспорта
	}
	if !resp.Delivered {
		return fmt.Errorf("delivery failed: %s", resp.Error)
	}
	return nil
}

func (c *Client) Close() error { return c.conn.Close() }
