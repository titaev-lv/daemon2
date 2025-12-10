package db

import (
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"fmt"
	"os"
	"time"

	"ctdaemon/internal/logger"

	"github.com/go-sql-driver/mysql"
)

type MySQLDriver struct {
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

func (m *MySQLDriver) Connect() error {
	dsn := m.User + ":" + m.Pass + "@tcp(" + m.Host + ":" + itoa(m.Port) + ")/" + m.Database + "?parseTime=true"

	// Add connection timeout if specified
	if m.ConnectTimeout > 0 {
		dsn += fmt.Sprintf("&timeout=%ds", int(m.ConnectTimeout.Seconds()))
	}

	// Configure TLS if enabled
	if m.UseTLS {
		tlsConfig, err := m.buildTLSConfig()
		if err != nil {
			return fmt.Errorf("failed to build TLS config: %w", err)
		}

		if err := mysql.RegisterTLSConfig("custom", tlsConfig); err != nil {
			return fmt.Errorf("failed to register TLS config: %w", err)
		}

		dsn += "&tls=custom"
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	m.DB = db

	if err := m.Ping(); err != nil {
		return err
	}

	if m.UseTLS {
		logger.Get("db").Info("Database connection with TLS certificates established successfully")
	}

	return nil
}

// buildTLSConfig creates a TLS configuration from certificate files
func (m *MySQLDriver) buildTLSConfig() (*tls.Config, error) {
	// Load CA certificate
	caCert, err := os.ReadFile(m.CACert)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA certificate: %w", err)
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to parse CA certificate")
	}

	// Load client certificate and key
	clientCert, err := tls.LoadX509KeyPair(m.ClientCert, m.ClientKey)
	if err != nil {
		return nil, fmt.Errorf("failed to load client certificate and key: %w", err)
	}

	return &tls.Config{
		RootCAs:            caCertPool,
		Certificates:       []tls.Certificate{clientCert},
		InsecureSkipVerify: m.TLSSkipVerify,
	}, nil
}

func (m *MySQLDriver) Close() error {
	if m.DB != nil {
		return m.DB.Close()
	}
	return nil
}

func (m *MySQLDriver) Ping() error {
	return m.DB.Ping()
}

func itoa(i int) string {
	return fmt.Sprintf("%d", i)
}
