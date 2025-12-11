// Package logger предоставляет единую систему логирования для всего приложения
// Использует стандартный Go slog (structured logging) для удобного анализа логов
// Поддерживает:
// - Разные уровни логирования (debug, info, warn, error)
// - Ротацию файлов по размеру с добавлением timestamp
// - Разные логгеры для разных компонентов (main, db, trade, orderbook и т.д.)
package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Глобальные переменные для системы логирования
var (
	// Log - основной логгер для всего приложения
	Log *slog.Logger
	// Trade - специальный логгер для торговых операций (может писать в отдельный файл)
	Trade *slog.Logger
	// logLevel - текущий уровень логирования (debug, info, warn, error)
	logLevel slog.Level
	// logDir - папка где хранятся логи
	logDir string
	// logFiles - map логгеров по имени (для разных компонентов)
	// Ключ: имя модуля (main, db, trade и т.д.)
	// Значение: io.WriteCloser для записи логов
	logFiles  map[string]io.WriteCloser
	fileMutex sync.RWMutex
	// maxLogSize - максимальный размер одного лог файла в байтах
	// При достижении размера файл ротируется
	maxLogSize int64
)

// plainTextHandler - пользовательский handler для slog
// Выводит логи в простом текстовом формате вместо JSON
type plainTextHandler struct {
	w      io.WriteCloser
	level  slog.Level
	module string
}

// rotatedFile - обертка вокруг файла с поддержкой ротации
// Автоматически ротирует файл при достижении максимального размера
type rotatedFile struct {
	file      *os.File
	filePath  string
	fileSize  int64
	maxSize   int64
	fileMutex sync.Mutex
}

// Write - записывает данные в файл с проверкой на ротацию
// Если размер файла + новые данные > maxSize, ротирует файл перед записью
func (rf *rotatedFile) Write(p []byte) (int, error) {
	rf.fileMutex.Lock()
	defer rf.fileMutex.Unlock()

	// Проверяем нужна ли ротация
	if rf.fileSize+int64(len(p)) > rf.maxSize {
		// Ротируем файл (переименовываем старый, создаем новый)
		if err := rf.rotate(); err != nil {
			// Если ротация не удалась, пытаемся все равно записать
			n, _ := rf.file.Write(p)
			rf.fileSize += int64(n)
			return n, nil
		}
	}

	// Записываем данные в файл
	n, err := rf.file.Write(p)
	rf.fileSize += int64(n)
	return n, err
}

// rotate - выполняет ротацию файла логов
// Переименовывает текущий файл в backup с timestamp и создает новый
func (rf *rotatedFile) rotate() error {
	// Закрываем текущий файл
	if err := rf.file.Close(); err != nil {
		return err
	}

	// Создаем резервное имя файла с timestamp
	// Пример: debug.2023-12-11_15-04-05.log
	timestamp := time.Now().Format("20060102_150405")
	dir := filepath.Dir(rf.filePath)
	name := filepath.Base(rf.filePath)
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	backupPath := filepath.Join(dir, fmt.Sprintf("%s.%s%s", base, timestamp, ext))

	// Переименовываем текущий файл в резервный
	if err := os.Rename(rf.filePath, backupPath); err != nil {
		return err
	}

	// Открываем новый файл для логирования
	f, err := os.OpenFile(rf.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	// Обновляем файловый дескриптор и обнуляем счетчик размера
	rf.file = f
	rf.fileSize = 0
	return nil
}

// Close - закрывает файл логирования
func (rf *rotatedFile) Close() error {
	rf.fileMutex.Lock()
	defer rf.fileMutex.Unlock()
	return rf.file.Close()
}

// Enabled - проверяет должен ли этот level логироваться
// Используется slog для фильтрации логов по уровню важности
func (h *plainTextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *plainTextHandler) Handle(ctx context.Context, r slog.Record) error {
	// Format: YYYY-MM-DD HH:MM:SS.000000 [LEVEL] [module] message [key=value...]
	timeStr := r.Time.Format("2006-01-02 15:04:05.000000")
	levelStr := strings.ToUpper(r.Level.String())
	msg := r.Message
	module := h.module // Use module from handler

	// Extract other attributes
	var otherAttrs []string
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "module" {
			return true // Skip, use handler's module instead
		} else if a.Key != slog.TimeKey && a.Key != slog.MessageKey {
			value := fmt.Sprint(a.Value.Any())
			otherAttrs = append(otherAttrs, fmt.Sprintf("%s=%s", a.Key, value))
		}
		return true
	})

	// Format: timestamp [LEVEL] [module] message [additional attrs]
	output := fmt.Sprintf("%s [%s] [%s] %s", timeStr, levelStr, module, msg)

	if len(otherAttrs) > 0 {
		output += " " + strings.Join(otherAttrs, " ")
	}

	output += "\n"

	// Handle rotation for rotatedFile
	switch w := h.w.(type) {
	case *rotatedFile:
		_, err := w.Write([]byte(output))
		return err
	default:
		_, err := io.WriteString(h.w, output)
		return err
	}
}

func (h *plainTextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	// Create a new handler with the same settings
	newH := &plainTextHandler{w: h.w, level: h.level, module: h.module}
	// Extract module if it's in the attrs
	for _, a := range attrs {
		if a.Key == "module" {
			newH.module = fmt.Sprint(a.Value.Any())
		}
	}
	return newH
}

func (h *plainTextHandler) WithGroup(name string) slog.Handler {
	return h
}

func init() {
	// Инициализируем глобальный map для логгеров
	logFiles = make(map[string]io.WriteCloser)
}

// Init - инициализирует систему логирования с указанными параметрами
// levelStr: "debug", "info", "warn", "error"
// dir: папка для логов
// maxFileSizeMB: максимальный размер одного файла логов
// Создает папку если ее нет и устанавливает основной логгер
func Init(levelStr, dir string, maxFileSizeMB int) error {
	// Создаем папку для логов если ее еще нет
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	logDir = dir
	// Переводим размер из мегабайт в байты
	maxLogSize = int64(maxFileSizeMB) * 1024 * 1024

	// Парсим строку уровня логирования в slog.Level
	// Поддерживаем: debug, info, warn/warning, error
	switch strings.ToLower(levelStr) {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	// Error Log (General)
	// Открываем файл для error логов
	// os.O_APPEND: добавляет в конец файла
	// os.O_CREATE: создает файл если его нет
	// os.O_WRONLY: открывает только для записи
	errorLogFile, err := os.OpenFile(filepath.Join(filepath.Clean(dir), "error.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	// Оборачиваем в rotatedFile для автоматической ротации
	errorRotated := &rotatedFile{
		file:     errorLogFile,
		filePath: filepath.Join(filepath.Clean(dir), "error.log"),
		maxSize:  maxLogSize,
	}
	// Получаем текущий размер файла (вдруг были логи до этого запуска)
	if info, err := errorLogFile.Stat(); err == nil {
		errorRotated.fileSize = info.Size()
	}
	logFiles["error"] = errorRotated

	// Открываем отдельный файл для торговых логов (для удобства анализа)
	tradeLogFile, err := os.OpenFile(filepath.Join(filepath.Clean(dir), "trade.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	// Оборачиваем в rotatedFile для автоматической ротации
	tradeRotated := &rotatedFile{
		file:     tradeLogFile,
		filePath: filepath.Join(filepath.Clean(dir), "trade.log"),
		maxSize:  maxLogSize,
	}
	// Получаем текущий размер файла
	if info, err := tradeLogFile.Stat(); err == nil {
		tradeRotated.fileSize = info.Size()
	}
	logFiles["trade"] = tradeRotated

	// Создаем глобальные логгеры с пользовательским handler для простого текстового формата
	// (вместо JSON который использует стандартный slog)
	Log = slog.New(&plainTextHandler{w: errorRotated, level: logLevel})
	Trade = slog.New(&plainTextHandler{w: tradeRotated, level: logLevel})

	return nil
}

// Get - возвращает логгер для конкретного модуля
// module: имя модуля (main, db, trade, orderbook и т.д.)
// Используется для идентификации источника логов: "2023-12-11 15:04:05 [INFO] [db] Connection established"
func Get(module string) *slog.Logger {
	// Если логгер не инициализирован (что странно), возвращаем запасной вариант в stdout
	if Log == nil {
		return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}
	// Добавляем поле "module" ко всем логам из этого логгера
	return Log.With("module", module)
}

// GetTrade - возвращает торговый логгер с контекстом модуля
func GetTrade(module string) *slog.Logger {
	if Trade == nil {
		return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}
	return Trade.With("module", module)
}

// Debug - логирует debug сообщение
// Используется для детальной отладки на уровне разработчика
// Содержит очень много информации, выключается в production
func Debug(msg string, args ...any) {
	if Log != nil {
		Log.Debug(msg, args...)
	}
}

// Info - логирует информационное сообщение
// Используется для основных событий (запуск, подключение, обновление и т.д.)
// Рекомендуемый уровень для production
func Info(msg string, args ...any) {
	if Log != nil {
		Log.Info(msg, args...)
	}
}

// Warn - логирует предупреждение
// Используется когда произойдет что-то неожиданное но не критичное
// Например: потеря соединения, повторное подключение, задержка в обработке
func Warn(msg string, args ...any) {
	if Log != nil {
		Log.Warn(msg, args...)
	}
}

// Error - логирует ошибку
// Используется при критичных ошибках которые требуют внимания
// Например: падение database соединения, некорректные данные, неудачное исполнение ордера
func Error(msg string, args ...any) {
	if Log != nil {
		Log.Error(msg, args...)
	}
}

// TradeInfo - логирует информацию о торговле в отдельный файл
// Все торговые события записываются в trade.log для анализа стратегий
// Пример: "Opened position BTC/USDT, entry price 45000"
func TradeInfo(msg string, args ...any) {
	if Trade != nil {
		Trade.Info(msg, args...)
	}
}

// TradeWarn - логирует предупреждение о торговле в отдельный файл
// Используется для проблем в торговле которые могут повлиять на результат
// Пример: "Position margin approaching liquidation level"
func TradeWarn(msg string, args ...any) {
	if Trade != nil {
		Trade.Warn(msg, args...)
	}
}

// TradeError - логирует критичную ошибку о торговле в отдельный файл
// Используется для критичных ошибок в торговле которые требуют немедленного внимания
// Пример: "Failed to place buy order for BTC/USDT"
func TradeError(msg string, args ...any) {
	if Trade != nil {
		Trade.Error(msg, args...)
	}
}

// Close - закрывает все открытые файлы логирования
// Вызывается при завершении приложения для корректного закрытия файлов
// Гарантирует что все логи записаны на диск перед выходом
func Close() error {
	fileMutex.Lock()
	defer fileMutex.Unlock()

	var lastErr error
	// Закрываем все открытые файлы логов
	for name, f := range logFiles {
		if err := f.Close(); err != nil {
			lastErr = err
		}
		delete(logFiles, name)
	}
	return lastErr
}

// GetLevel - возвращает текущий уровень логирования
// Используется для проверки какой уровень включен без переинициализации
func GetLevel() slog.Level {
	return logLevel
}

// GetLogDir returns the log directory
func GetLogDir() string {
	return logDir
}

// SetMaxLogSize sets the maximum log file size before rotation
func SetMaxLogSize(size int64) {
	maxLogSize = size
}
