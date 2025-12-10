package main

import (
	// "encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"ctdaemon/internal/api"
	"ctdaemon/internal/config"
	"ctdaemon/internal/db"
	"ctdaemon/internal/logger"
	"ctdaemon/internal/manager"
	"ctdaemon/internal/state"
)

const (
	Version = "2.0.1"
)

func main() {
	configFile := flag.String("c", "conf/config.ini", "Path to configuration file")
	flag.Parse()
	// 1. Load Config
	cfg, err := config.Load(*configFile)
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// 2. Init Logger
	if err := logger.Init(cfg.Log.Level, cfg.Log.Dir, cfg.Log.MaxFileSizeMB); err != nil {
		fmt.Printf("Failed to init logger: %+v\n", err)
		os.Exit(1)
	}
	defer logger.Close()
	log := logger.Get("main")
	log.Info("\n\n")
	log.Info("==========================================================")
	log.Info("INIT START ctdaemon", "version", Version)
	log.Info("Starting ctdaemon", "config", *configFile)

	// 3. Init Database
	if err := db.Init(cfg.Database); err != nil {
		log.Error("Failed to init database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// 4. Init Manager
	mgr := manager.New(cfg)

	// 5. Auto-start if state says it should be running
	if state.GetInstance().IsRunning() {
		log.Info("Auto-starting daemon based on saved state")
		if err := mgr.Start(); err != nil {
			log.Error("Failed to auto-start daemon", "error", err)
		}
	}

	// 6. Init API Server
	apiServer := api.New(cfg.Server, mgr, Version)
	go func() {
		if err := apiServer.Start(); err != nil {
			log.Error("API server failed", "error", err)
			os.Exit(1)
		}
	}()

	// 7. Handle Signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Wait for signal
	sig := <-sigChan
	log.Info("Received signal, shutting down...", "signal", sig)

	// 8. Graceful Shutdown
	if err := mgr.Stop(); err != nil {
		log.Error("Error during shutdown", "error", err)
	}
	log.Info("Shutdown complete")

	// jsonData, _ := json.MarshalIndent(cfg, "", "  ")
	// fmt.Println(string(jsonData))
}
