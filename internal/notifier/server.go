package notifier

import (
	"context"
	"fmt"

	"github.com/WOLFraMix/pricepulse/internal/pb"
)

// Server реализует Notifier из контракта. Встраиваем
// UnimplementedNotifierServer: это рекомендованный gRPC паттерн —
// при добавлении методов в .proto старый бинарник не падает,
// а возвращает «не реализовано», пока не перепишем.
type Server struct {
	pb.UnimplementedNotifierServer
	tg *Telegram
}

func NewServer(tg *Telegram) *Server {
	return &Server{tg: tg}
}

func (s *Server) SendAlert(ctx context.Context, req *pb.AlertRequest) (*pb.AlertReply, error) {
	text := fmt.Sprintf(
		"PricePulse: цена упала!\n\n%s\nТекущая: %.2f %s (цель: %.2f)\n%s",
		req.Title, req.Price, req.Currency, req.TargetPrice, req.Url,
	)
	if err := s.tg.Send(ctx, text); err != nil {
		// Ошибка доставки — это не ошибка gRPC-вызова: вызов дошёл
		// и честно отчитался в поле error. Клиент сам решит, что делать.
		return &pb.AlertReply{Delivered: false, Error: err.Error()}, nil
	}
	return &pb.AlertReply{Delivered: true}, nil
}
