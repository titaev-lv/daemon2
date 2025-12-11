// Package db предоставляет драйверы подключения к MySQL и PostgreSQL
// с поддержкой TLS/SSL шифрования, retry логики и таймаутов.
//
// Использование:
//
//driver := &MySQLDriver{
//Host:     "localhost",
//Port:     3306,
//User:     "root",
//Pass:     "password",
//Database: "myapp",
//UseTLS:   true,
//}
//if err := driver.Connect(); err != nil {
//log.Fatal(err)
//}
//defer driver.Close()
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

// MySQLDriver инкапсулирует MySQL подключение с поддержкой TLS/SSL и конфигурацией пула соединений
//
// Поля:
//   - DB: *sql.DB - основное подключение к БД
//   - Host: хост MySQL сервера (например: localhost или mysql.example.com)
//   - Port: порт MySQL (обычно 3306)
//   - User: имя пользователя для аутентификации
//   - Pass: пароль для аутентификации
//   - Database: название БД для подключения
//   - UseTLS: включить ли TLS/SSL шифрование (рекомендуется true для production)
//   - CACert: путь к сертификату CA для проверки сертификата сервера
//   - ClientCert: путь к сертификату клиента для взаимной аутентификации
//   - ClientKey: путь к приватному ключу клиента
//   - TLSSkipVerify: пропустить ли проверку сертификата (ОПАСНО! только для разработки)
//   - ConnectTimeout: таймаут подключения (например: 10 * time.Second)
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

// Connect устанавливает подключение к MySQL серверу
//
// Алгоритм:
// 1. Конструирует строку подключения (DSN) из параметров хоста, порта, пользователя и пароля
// 2. Добавляет параметр parseTime=true для корректной работы с временем
// 3. Если указан ConnectTimeout, добавляет его в DSN (секунды)
// 4. Если UseTLS=true:
//    - Создает TLS конфигурацию из сертификатов
//    - Регистрирует конфиг в драйвере MySQL
//    - Добавляет параметр tls=custom в DSN
// 5. Открывает подключение к БД
// 6. Настраивает пул соединений:
//    - MaxOpenConns=20: максимум 20 одновременных соединений
//    - MaxIdleConns=5: максимум 5 неиспользуемых соединений в кэше
// 7. Проверяет подключение через Ping()
// 8. Логирует успешное подключение
//
// Возвращает:
//   - nil при успешном подключении
//   - ошибку если что-то пошло не так
func (m *MySQLDriver) Connect() error {
// Конструируем DSN (Data Source Name) для подключения
// Формат: user:password@tcp(host:port)/database?options
dsn := m.User + ":" + m.Pass + "@tcp(" + m.Host + ":" + itoa(m.Port) + ")/" + m.Database + "?parseTime=true"

// Добавляем таймаут подключения если он указан
// Таймаут определяет максимальное время на установку соединения
if m.ConnectTimeout > 0 {
dsn += fmt.Sprintf("&timeout=%ds", int(m.ConnectTimeout.Seconds()))
}

// Настраиваем TLS/SSL если включено (рекомендуется для production)
// TLS шифрует трафик между приложением и БД
if m.UseTLS {
tlsConfig, err := m.buildTLSConfig()
if err != nil {
return fmt.Errorf("failed to build TLS config: %w", err)
}

// Регистрируем TLS конфиг в драйвере MySQL под именем "custom"
if err := mysql.RegisterTLSConfig("custom", tlsConfig); err != nil {
return fmt.Errorf("failed to register TLS config: %w", err)
}

// Указываем в DSN использовать зарегистрированный TLS конфиг
dsn += "&tls=custom"
}

// Открываем подключение к MySQL
db, err := sql.Open("mysql", dsn)
if err != nil {
return err
}

// Настраиваем пул соединений для оптимальной производительности
// MaxOpenConns: максимум одновременных соединений
// MaxIdleConns: максимум соединений в кэше (для переиспользования)
db.SetMaxOpenConns(20)
db.SetMaxIdleConns(5)
m.DB = db

// Проверяем что подключение работает
if err := m.Ping(); err != nil {
return err
}

// Логируем успешное подключение с TLS
if m.UseTLS {
logger.Get("db").Info("Database connection with TLS certificates established successfully")
}

return nil
}

// buildTLSConfig создает TLS конфигурацию из файлов сертификатов
//
// Процесс:
// 1. Читает сертификат CA из файла (m.CACert)
// 2. Парсит сертификат в x509 формат
// 3. Создает пул корневых сертификатов (для проверки сертификата сервера)
// 4. Загружает сертификат и приватный ключ клиента (взаимная аутентификация)
// 5. Возвращает конфигурацию TLS
//
// Сертификаты:
//   - CACert: корневой сертификат (для проверки сертификата сервера БД)
//   - ClientCert: сертификат клиента (для идентификации перед БД)
//   - ClientKey: приватный ключ клиента (должен быть защищен от посторонних)
//
// Безопасность:
//   - Если TLSSkipVerify=true: не проверяем сертификат сервера (ОПАСНО!)
//   - Если TLSSkipVerify=false: проверяем что сертификат подписан нашей CA (безопасно)
//
// Возвращает:
//   - *tls.Config: готовая конфигурация для использования
//   - error: если не удалось прочитать или расспарсить сертификаты
func (m *MySQLDriver) buildTLSConfig() (*tls.Config, error) {
// Читаем файл с сертификатом CA (Certificate Authority)
// CA используется для проверки что сертификат сервера подписан доверенной организацией
caCert, err := os.ReadFile(m.CACert)
if err != nil {
return nil, fmt.Errorf("failed to read CA certificate: %w", err)
}

// Создаем пул сертификатов и добавляем CA сертификат
// Пул используется для проверки цепи сертификатов
caCertPool := x509.NewCertPool()
if !caCertPool.AppendCertsFromPEM(caCert) {
return nil, fmt.Errorf("failed to parse CA certificate")
}

// Загружаем сертификат клиента и его приватный ключ
// Это используется для взаимной аутентификации (mTLS)
// Сервер БД может требовать чтобы клиент предоставил свой сертификат
clientCert, err := tls.LoadX509KeyPair(m.ClientCert, m.ClientKey)
if err != nil {
return nil, fmt.Errorf("failed to load client certificate and key: %w", err)
}

// Возвращаем TLS конфигурацию
return &tls.Config{
RootCAs:            caCertPool,              // Пул CA для проверки сертификата сервера
Certificates:       []tls.Certificate{clientCert}, // Сертификат клиента для аутентификации
InsecureSkipVerify: m.TLSSkipVerify,        // Пропустить проверку сертификата (только разработка!)
}, nil
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
func (m *MySQLDriver) Close() error {
if m.DB != nil {
return m.DB.Close()
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
func (m *MySQLDriver) Ping() error {
return m.DB.Ping()
}

// itoa преобразует целое число в строку
// Хелпер функция для конструирования DSN
func itoa(i int) string {
return fmt.Sprintf("%d", i)
}
