package config

import (
	"fmt"

	"github.com/go-ini/ini"
)

type Config struct {
	Database  DatabaseConfig
	Server    ServerConfig
	Log       LogConfig
	Trade     TradeConfig
	OrderBook OrderBookConfig
}

type OrderBookConfig struct {
	DebugLogRaw bool
	DebugLogMsg bool
}

type DatabaseConfig struct {
	Type          string // "mysql" или "postgres"
	User          string
	Password      string
	Host          string
	Port          int
	Name          string
	UseTLS        bool   // Use TLS/SSL for connection
	CACert        string // Path to CA certificate
	ClientCert    string // Path to client certificate
	ClientKey     string // Path to client key
	TLSSkipVerify bool   // Skip certificate verification (insecure, for IP addresses)
}

type ServerConfig struct {
	Port      int
	StateFile string
}

type LogConfig struct {
	Level         string
	Dir           string
	MaxFileSizeMB int
}

type TradeConfig struct {
	UpdateInterval int
}

func Load(path string) (*Config, error) {
	cfg, err := ini.Load(path)
	if err != nil {
		return nil, fmt.Errorf("failed to load config file: %w", err)
	}

	c := &Config{}

	// Database
	c.Database.Type = cfg.Section("database").Key("type").MustString("mysql")
	c.Database.User = cfg.Section("database").Key("user").String()
	c.Database.Password = cfg.Section("database").Key("password").String()
	c.Database.Host = cfg.Section("database").Key("host").String()
	c.Database.Port = cfg.Section("database").Key("port").MustInt(3306)
	c.Database.Name = cfg.Section("database").Key("name").String()
	c.Database.UseTLS = cfg.Section("database").Key("use_tls").MustBool(false)
	c.Database.CACert = cfg.Section("database").Key("ca_cert").String()
	c.Database.ClientCert = cfg.Section("database").Key("client_cert").String()
	c.Database.ClientKey = cfg.Section("database").Key("client_key").String()
	c.Database.TLSSkipVerify = cfg.Section("database").Key("tls_skip_verify").MustBool(false)

	// Server
	c.Server.Port = cfg.Section("server").Key("port").MustInt(8080)
	c.Server.StateFile = cfg.Section("server").Key("state_file").MustString("state.json")

	// Log
	c.Log.Level = cfg.Section("log").Key("level").MustString("info")
	c.Log.Dir = cfg.Section("log").Key("dir").MustString("./logs")
	c.Log.MaxFileSizeMB = cfg.Section("log").Key("max_file_size_mb").MustInt(10)

	// Trade
	c.Trade.UpdateInterval = cfg.Section("trade").Key("update_interval").MustInt(5)

	// OrderBook
	c.OrderBook.DebugLogRaw = cfg.Section("orderbook").Key("debug_log_raw").MustBool(false)
	c.OrderBook.DebugLogMsg = cfg.Section("orderbook").Key("debug_log_msg").MustBool(false)

	return c, nil
}
