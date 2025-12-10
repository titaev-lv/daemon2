package db

import (
	"ctdaemon/internal/config"
	"ctdaemon/internal/logger"
	"fmt"
	"math"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
)

var driver DBDriver

// ConnectWithRetry attempts to connect to the database with exponential backoff
func ConnectWithRetry(cfg config.DatabaseConfig) error {
	log := logger.Get("db")
	maxRetries := cfg.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 1 // At least one attempt
	}

	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		log.Info("Database connection attempt", "attempt", attempt, "max_retries", maxRetries)

		// Create the appropriate driver
		var d DBDriver

		if cfg.Type == "postgres" || cfg.Type == "postgresql" {
			d = &PostgresDriver{
				Host:           cfg.Host,
				Port:           cfg.Port,
				User:           cfg.User,
				Pass:           cfg.Password,
				Database:       cfg.Name,
				UseTLS:         cfg.UseTLS,
				CACert:         cfg.CACert,
				ClientCert:     cfg.ClientCert,
				ClientKey:      cfg.ClientKey,
				TLSSkipVerify:  cfg.TLSSkipVerify,
				ConnectTimeout: time.Duration(cfg.ConnectTimeoutSec) * time.Second,
			}
		} else {
			d = &MySQLDriver{
				Host:           cfg.Host,
				Port:           cfg.Port,
				User:           cfg.User,
				Pass:           cfg.Password,
				Database:       cfg.Name,
				UseTLS:         cfg.UseTLS,
				CACert:         cfg.CACert,
				ClientCert:     cfg.ClientCert,
				ClientKey:      cfg.ClientKey,
				TLSSkipVerify:  cfg.TLSSkipVerify,
				ConnectTimeout: time.Duration(cfg.ConnectTimeoutSec) * time.Second,
			}
		}

		// Try to connect
		if err := d.Connect(); err != nil {
			lastErr = err
			log.Warn("Database connection failed", "attempt", attempt, "error", err)

			// Calculate backoff interval
			if attempt < maxRetries {
				backoffInterval := calculateBackoffInterval(attempt)
				log.Info("Waiting before retry", "attempt", attempt, "backoff_seconds", backoffInterval)
				time.Sleep(time.Duration(backoffInterval) * time.Second)
			}
			continue
		}

		// Success
		driver = d
		log.Info("Database connected successfully", "attempt", attempt, "type", cfg.Type)
		return nil
	}

	return fmt.Errorf("failed to connect to database after %d attempts: %w", maxRetries, lastErr)
}

// calculateBackoffInterval uses exponential backoff with a cap
// Starts at 1 second after 10 attempts, then increases exponentially
func calculateBackoffInterval(attempt int) int {
	if attempt <= 10 {
		return 0 // No wait for first 10 attempts
	}

	// After 10 attempts, start exponential backoff: 1, 2, 4, 8, 16...
	backoffMultiplier := attempt - 10
	interval := int(math.Pow(2, float64(backoffMultiplier-1)))

	// Cap at 300 seconds (5 minutes)
	if interval > 300 {
		interval = 300
	}

	return interval
}

func Init(cfg config.DatabaseConfig) error {
	// Validate TLS configuration if enabled
	if cfg.UseTLS {
		if err := validateTLSConfig(cfg); err != nil {
			return err
		}
	}

	return ConnectWithRetry(cfg)
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
