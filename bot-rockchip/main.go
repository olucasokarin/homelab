package main

import (
	"log"
	
	"rockchip-bot/bot"
	"rockchip-bot/config"
)

func main() {
	log.Println("Starting TeleBot...")
	
	// Load Configuration
	config.Load()
	
	// Start API Server for the Dashboard (Port 5001)
	go bot.StartAPI("5001")

	// Start Bot (blocks)
	bot.Start()
}
