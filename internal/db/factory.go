package db

import (
	"ctdaemon/internal/config"
	"ctdaemon/internal/logger"
	"fmt"
	"os"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
)

var driver DBDriver

func Init(cfg config.DatabaseConfig) error {
	// Validate TLS configuration if enabled
	if cfg.UseTLS {
		if err := validateTLSConfig(cfg); err != nil {
			return err
		}
	}

	switch cfg.Type {
	case "postgres", "postgresql":
		d := &PostgresDriver{
			Host:          cfg.Host,
			Port:          cfg.Port,
			User:          cfg.User,
			Pass:          cfg.Password,
			Database:      cfg.Name,
			UseTLS:        cfg.UseTLS,
			CACert:        cfg.CACert,
			ClientCert:    cfg.ClientCert,
			ClientKey:     cfg.ClientKey,
			TLSSkipVerify: cfg.TLSSkipVerify,
		}
		if err := d.Connect(); err != nil {
			return fmt.Errorf("failed to connect to PostgreSQL: %w", err)
		}
		driver = d

	case "mysql", "":
		// mysql по умолчанию
		d := &MySQLDriver{
			Host:          cfg.Host,
			Port:          cfg.Port,
			User:          cfg.User,
			Pass:          cfg.Password,
			Database:      cfg.Name,
			UseTLS:        cfg.UseTLS,
			CACert:        cfg.CACert,
			ClientCert:    cfg.ClientCert,
			ClientKey:     cfg.ClientKey,
			TLSSkipVerify: cfg.TLSSkipVerify,
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

// validateTLSConfig checks that all required TLS files exist and are readable
func validateTLSConfig(cfg config.DatabaseConfig) error {
	files := map[string]string{
		"ca_cert":     cfg.CACert,
		"client_cert": cfg.ClientCert,
		"client_key":  cfg.ClientKey,
	}

	for name, path := range files {
		if path == "" {
			return fmt.Errorf("TLS is enabled but %s is not specified", name)
		}

		// Check if file exists
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("%s file not found: %s", name, path)
			}
			return fmt.Errorf("cannot access %s file at %s: %w", name, path, err)
		}

		// Try to read file to check permissions
		if _, err := os.ReadFile(path); err != nil {
			return fmt.Errorf("cannot read %s file at %s: %w", name, path, err)
		}
	}

	return nil
}

func Close() {
	// Implementation for closing the database connection
	if driver != nil {
		driver.Close()
	}
}
