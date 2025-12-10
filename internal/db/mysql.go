package db

import (
	"database/sql"
	"fmt"
)

type MySQLDriver struct {
	DB       *sql.DB
	Host     string
	Port     int
	User     string
	Pass     string
	Database string
}

func (m *MySQLDriver) Connect() error {
	dsn := m.User + ":" + m.Pass + "@tcp(" + m.Host + ":" + itoa(m.Port) + ")/" + m.Database + "?parseTime=true"
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	m.DB = db
	return m.Ping()
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
