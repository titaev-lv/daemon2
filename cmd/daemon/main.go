package main

import (
	// "encoding/json"
	"flag"
	"fmt"
	"os"

	"ctdaemon/internal/config"
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

	_ = cfg

	// jsonData, _ := json.MarshalIndent(cfg, "", "  ")
	// fmt.Println(string(jsonData))
}
