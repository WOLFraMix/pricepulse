package main

import (
	"log"
	"net"

	"google.golang.org/grpc"

	"github.com/WOLFraMix/pricepulse/internal/config"
	"github.com/WOLFraMix/pricepulse/internal/notifier"
	"github.com/WOLFraMix/pricepulse/internal/pb"
)

func main() {
	cfg := config.Load()
	if cfg.TelegramToken == "" || cfg.TelegramChat == "" {
		log.Fatalf("notifier: required PW_TG_TOKEN and PW_TG_CHAT")
	}

	lis, err := net.Listen("tcp", cfg.NotifierAddr)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterNotifierServer(grpcServer,
		notifier.NewServer(notifier.NewTelegram(cfg.TelegramToken, cfg.TelegramChat)))

	log.Printf("notifier: gRPC on %s", cfg.NotifierAddr)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
