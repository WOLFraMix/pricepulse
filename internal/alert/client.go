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

// Client — обёртка над сгенерированным стабом.
type Client struct {
	conn *grpc.ClientConn
	api  pb.NotifierClient
}

// New создаёт клиента. grpc.NewClient ленив: реальное соединение
// откроется при первом вызове, поэтому порядок запуска сервисов не важен.
func New(addr string) (*Client, error) {
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return &Client{conn: conn, api: pb.NewNotifierClient(conn)}, nil
}

// Alert отправляет уведомление. reason — человеческая причина срабатывания.
func (c *Client) Alert(ctx context.Context, task model.CheckTask,
	title, currency string, price float64, reason string) error {

	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	resp, err := c.api.SendAlert(ctx, &pb.AlertRequest{
		ProductId:   task.ProductID,
		Title:       title,
		Url:         task.URL,
		Price:       price,
		Currency:    currency,
		TargetPrice: task.TargetPrice,
		Reason:      reason,
	})
	if err != nil {
		return err
	}
	if !resp.Delivered {
		return fmt.Errorf("delivery failed: %s", resp.Error)
	}
	return nil
}

func (c *Client) Close() error { return c.conn.Close() }
