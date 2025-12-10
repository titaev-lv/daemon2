package db

import (
	"daemon2/internal/config"
	"daemon2/internal/logger"
	"fmt"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
)

var driver DBDriver

func Init(cfg config.DatabaseConfig) error {
	switch cfg.Type {
	case "postgres", "postgresql":
		d := &PostgresDriver{
			Host:     cfg.Host,
			Port:     cfg.Port,
			User:     cfg.User,
			Pass:     cfg.Password,
			Database: cfg.Name,
		}
		if err := d.Connect(); err != nil {
			return fmt.Errorf("failed to connect to PostgreSQL: %w", err)
		}
		driver = d

	case "mysql", "":
		// mysql по умолчанию
		d := &MySQLDriver{
			Host:     cfg.Host,
			Port:     cfg.Port,
			User:     cfg.User,
			Pass:     cfg.Password,
			Database: cfg.Name,
		}
		if err := d.Connect(); err != nil {
			return fmt.Errorf("failed to connect to MySQL: %w", err)
		}
		driver = d

	default:
		return fmt.Errorf("unsupported database type: %s", cfg.Type)
	}

	logger.Get("db").Info("Database initialized", "type", cfg.Type, "host", cfg.Host, "name", cfg.Name)
	return nil
}

func Close() {
	// Implementation for closing the database connection
	if driver != nil {
		driver.Close()
	}
}
