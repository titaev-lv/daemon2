package main

import (
	// "encoding/json"
	"flag"
	"fmt"
	"os"

	"ctdaemon/internal/config"
	"ctdaemon/internal/db"
	"ctdaemon/internal/logger"
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
	log.Info("Starting ctdaemon", "config", *configFile)

	// 3. Init Database
	if err := db.Init(cfg.Database); err != nil {
		log.Error("Failed to init database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// jsonData, _ := json.MarshalIndent(cfg, "", "  ")
	// fmt.Println(string(jsonData))
	os.Exit(0)
}
