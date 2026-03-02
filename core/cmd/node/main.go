package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"reserveos/core/node"
)

func main() {
	cfgPath := "config/node/node.json"
	if len(os.Args) > 1 {
		cfgPath = os.Args[1]
	}

	n, err := node.NewFromConfig(cfgPath)
	if err != nil {
		log.Fatalf("boot failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := n.Start(ctx); err != nil {
		log.Fatalf("start failed: %v", err)
	}

	sig := make(chan os.Signal, 2)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	_ = n.Stop(context.Background())
}
