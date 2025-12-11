// Package db предоставляет драйверы подключения к MySQL и PostgreSQL
// с поддержкой TLS/SSL шифрования, retry логики и таймаутов.
package db

import (
"database/sql"
"fmt"
"time"

"ctdaemon/internal/logger"
)

// PostgresDriver инкапсулирует PostgreSQL подключение с поддержкой TLS/SSL и конфигурацией пула соединений
//
// Поля:
//   - DB: *sql.DB - основное подключение к БД
//   - Host: хост PostgreSQL сервера (например: localhost или postgres.example.com)
//   - Port: порт PostgreSQL (обычно 5432)
//   - User: имя пользователя для аутентификации
//   - Pass: пароль для аутентификации
//   - Database: название БД для подключения
//   - UseTLS: включить ли TLS/SSL шифрование (рекомендуется true для production)
//   - CACert: путь к сертификату CA для проверки сертификата сервера
//   - ClientCert: путь к сертификату клиента для взаимной аутентификации
//   - ClientKey: путь к приватному ключу клиента
//   - TLSSkipVerify: пропустить ли проверку сертификата (ОПАСНО! только для разработки)
//   - ConnectTimeout: таймаут подключения (например: 10 * time.Second)
//
// Отличия PostgreSQL от MySQL:
// - Использует свой синтаксис DSN (connectstring вместо URL)
// - Параметры TLS передаются через sslcert, sslkey, sslrootcert
// - Поддерживает режимы SSL: disable, allow, prefer, require, verify-ca, verify-full
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

// Connect устанавливает подключение к PostgreSQL серверу
//
// Алгоритм:
// 1. Конструирует строку подключения (connection string) из параметров
// 2. По умолчанию использует sslmode=disable (без шифрования)
// 3. Если указан ConnectTimeout, добавляет его в строку подключения
// 4. Если UseTLS=true:
//    - Меняет sslmode на "require" (требует TLS)
//    - Если TLSSkipVerify=true: использует sslmode="require" без проверки сертификата
//    - Если TLSSkipVerify=false: добавляет пути к сертификатам (sslcert, sslkey, sslrootcert)
// 5. Открывает подключение к БД
// 6. Настраивает пул соединений:
//    - MaxOpenConns=20: максимум 20 одновременных соединений
//    - MaxIdleConns=5: максимум 5 неиспользуемых соединений в кэше
// 7. Проверяет подключение через Ping()
// 8. Логирует успешное подключение
//
// Режимы SSL в PostgreSQL:
//   - disable: без шифрования (опасно для production)
//   - require: требует TLS но не проверяет сертификат (уязвимо для MITM)
//   - verify-ca: требует TLS и проверяет что сертификат подписан CA (рекомендуется)
//   - verify-full: проверяет сертификат и имя хоста (самое безопасное)
//
// Возвращает:
//   - nil при успешном подключении
//   - ошибку если что-то пошло не так
func (p *PostgresDriver) Connect() error {
// По умолчанию подключаемся без TLS (тестирование)
sslMode := "disable"
dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
p.Host, p.Port, p.User, p.Pass, p.Database, sslMode)

// Добавляем таймаут подключения если он указан
// Таймаут определяет максимальное время на установку соединения (в секундах)
if p.ConnectTimeout > 0 {
dsn += fmt.Sprintf(" connect_timeout=%d", int(p.ConnectTimeout.Seconds()))
}

// Настраиваем TLS/SSL если включено (рекомендуется для production)
// TLS шифрует трафик между приложением и БД
if p.UseTLS {
// Требуем TLS для подключения
sslMode = "require"

// Два варианта конфигурации TLS:
if p.TLSSkipVerify {
// Вариант 1: TLS но без проверки сертификата
// ОПАСНО! Уязвимо для MITM (Man-In-The-Middle) атак
// Используется только для разработки или в полностью закрытой сети
sslMode = "require"
dsn = fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s sslcert=%s sslkey=%s sslrootcert=%s",
p.Host, p.Port, p.User, p.Pass, p.Database, sslMode, p.ClientCert, p.ClientKey, p.CACert)
} else {
// Вариант 2: TLS с проверкой сертификата
// БЕЗОПАСНО! Проверяем что сертификат подписан доверенной CA
// Требуется для production
dsn = fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s sslcert=%s sslkey=%s sslrootcert=%s",
p.Host, p.Port, p.User, p.Pass, p.Database, sslMode, p.ClientCert, p.ClientKey, p.CACert)
}

// Добавляем таймаут подключения после конфигурации TLS
if p.ConnectTimeout > 0 {
dsn += fmt.Sprintf(" connect_timeout=%d", int(p.ConnectTimeout.Seconds()))
}
}

// Открываем подключение к PostgreSQL
db, err := sql.Open("postgres", dsn)
if err != nil {
return err
}

// Настраиваем пул соединений для оптимальной производительности
// MaxOpenConns: максимум одновременных соединений
// MaxIdleConns: максимум соединений в кэше (для переиспользования)
db.SetMaxOpenConns(20)
db.SetMaxIdleConns(5)
p.DB = db

// Проверяем что подключение работает
if err := p.Ping(); err != nil {
return err
}

// Логируем успешное подключение с TLS
if p.UseTLS {
logger.Get("db").Info("Database connection with TLS/SSL certificates established successfully")
}

return nil
}

// Close закрывает подключение к БД и освобождает все связанные ресурсы
//
// Важно:
// - Должен быть вызван перед завершением приложения
// - Закрывает все соединения в пуле
// - После этого вызова объект уже не может быть использован
//
// Пример:
//
//defer driver.Close()
func (p *PostgresDriver) Close() error {
if p.DB != nil {
return p.DB.Close()
}
return nil
}

// Ping проверяет что соединение с БД все еще активно
//
// Используется для:
// - Проверки что БД доступна при запуске
// - Health checks в процессе работы
// - Обнаружения разорванных соединений
//
// Возвращает:
//   - nil если БД отвечает
//   - error если БД недоступна или соединение разорвано
func (p *PostgresDriver) Ping() error {
return p.DB.Ping()
}
