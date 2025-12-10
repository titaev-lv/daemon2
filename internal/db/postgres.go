package db

import (
	"database/sql"
	"fmt"
	"time"

	"ctdaemon/internal/logger"
)

type PostgresDriver struct {
	DB             *sql.DB
	Host           string
	Port           int
	User           string
	Pass           string
	Database       string
	UseTLS         bool
	CACert         string
	ClientCert     string
	ClientKey      string
	TLSSkipVerify  bool
	ConnectTimeout time.Duration
}

func (p *PostgresDriver) Connect() error {
	sslMode := "disable"
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		p.Host, p.Port, p.User, p.Pass, p.Database, sslMode)

	// Add connection timeout if specified
	if p.ConnectTimeout > 0 {
		dsn += fmt.Sprintf(" connect_timeout=%d", int(p.ConnectTimeout.Seconds()))
	}

	// Configure TLS if enabled
	if p.UseTLS {
		sslMode = "require"
		if p.TLSSkipVerify {
			sslMode = "require"
			// For skipping verification in PostgreSQL
			dsn = fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s sslcert=%s sslkey=%s sslrootcert=%s",
				p.Host, p.Port, p.User, p.Pass, p.Database, p.ClientCert, p.ClientKey, p.CACert)
		} else {
			dsn = fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s sslcert=%s sslkey=%s sslrootcert=%s",
				p.Host, p.Port, p.User, p.Pass, p.Database, sslMode, p.ClientCert, p.ClientKey, p.CACert)
		}

		// Add connection timeout after TLS config
		if p.ConnectTimeout > 0 {
			dsn += fmt.Sprintf(" connect_timeout=%d", int(p.ConnectTimeout.Seconds()))
		}
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	p.DB = db

	if err := p.Ping(); err != nil {
		return err
	}

	if p.UseTLS {
		logger.Get("db").Info("Database connection with TLS/SSL certificates established successfully")
	}

	return nil
}

func (p *PostgresDriver) Close() error {
	if p.DB != nil {
		return p.DB.Close()
	}
	return nil
}

func (p *PostgresDriver) Ping() error {
	return p.DB.Ping()
}
