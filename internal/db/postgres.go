package db

import (
	"database/sql"
	"fmt"
)

type PostgresDriver struct {
	DB       *sql.DB
	Host     string
	Port     int
	User     string
	Pass     string
	Database string
}

func (p *PostgresDriver) Connect() error {
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		p.Host, p.Port, p.User, p.Pass, p.Database)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	p.DB = db
	return p.Ping()
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
