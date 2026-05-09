package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"torrent-sniffer/internal/db"
	"torrent-sniffer/internal/sniffer"
	"torrent-sniffer/internal/torrent"
	"torrent-sniffer/internal/web"
)

func main() {
	log.Println("Starting Torrent Sniffer Service...")

	// Initialize Torrent Engine
	engine, err := torrent.NewEngine()
	if err != nil {
		log.Fatalf("Failed to initialize torrent engine: %v", err)
	}
	defer engine.Close()

	// Initialize Database
	database, err := db.Connect("sniffer.db")
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	// Initialize Sniffer Service (Concurrency limit: 10)
	snifferSvc := sniffer.NewService(engine, database, 10)

	// Initialize Web Handler
	handler := web.NewHandler(snifferSvc)

	// Setup HTTP Server
	mux := http.NewServeMux()
	mux.HandleFunc("/sniff", handler.Sniff)
	mux.HandleFunc("/history", handler.History)
	mux.HandleFunc("/queue", handler.Queue)
	mux.HandleFunc("/cancel", handler.Cancel)
	mux.HandleFunc("/delete", handler.Delete)
	mux.HandleFunc("/flush", handler.Flush)
	mux.HandleFunc("/notes", handler.SaveNotes)
	mux.HandleFunc("/health", web.Health)

	// Static Dashboard
	fs := http.FileServer(http.Dir("static"))
	mux.Handle("/", fs)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	server := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	// Graceful Shutdown
	go func() {
		log.Printf("Server listening on port %s", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exiting")
}
