package notifier

import (
	"context"
	"fmt"
	"log"

	"github.com/WOLFraMix/pricepulse/internal/pb"
)

// Server реализует Notifier из контракта.
type Server struct {
	pb.UnimplementedNotifierServer
	tg *Telegram
}

func NewServer(tg *Telegram) *Server {
	return &Server{tg: tg}
}

func (s *Server) SendAlert(ctx context.Context, req *pb.AlertRequest) (*pb.AlertReply, error) {
	log.Printf("notifier: alert for product %d: %.2f %s", req.ProductId, req.Price, req.Currency)
	text := fmt.Sprintf(
		"PricePulse: %s\n\n%s\nТекущая: %.2f %s (цель: %.2f)\n%s",
		req.Reason, req.Title, req.Price, req.Currency, req.TargetPrice, req.Url,
	)
	if err := s.tg.Send(ctx, text); err != nil {
		log.Printf("notifier: telegram send failed: %v", err)
		return &pb.AlertReply{Delivered: false, Error: err.Error()}, nil
	}
	log.Printf("notifier: delivered, product %d", req.ProductId)
	return &pb.AlertReply{Delivered: true}, nil
}
