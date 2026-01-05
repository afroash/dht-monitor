package main

import (
	"fmt"
	"log"

	"github.com/afroash/dht-monitor/internal/config"
)

func main() {
	cfg, err := config.LoadConfig("configs/sensor.local.yaml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	fmt.Println(cfg)
}
