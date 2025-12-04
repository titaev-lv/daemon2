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

var (
	Log       *slog.Logger
	Trade     *slog.Logger
	logLevel  slog.Level
	logDir    string
	logFiles  map[string]io.WriteCloser
	fileMutex sync.RWMutex
	// Rotation settings
	maxLogSize int64
)

// plainTextHandler is a custom slog handler for simple text format
type plainTextHandler struct {
	w      io.WriteCloser
	level  slog.Level
	module string
}

// rotatedFile wraps a file and handles rotation
type rotatedFile struct {
	file      *os.File
	filePath  string
	fileSize  int64
	maxSize   int64
	fileMutex sync.Mutex
}

func (rf *rotatedFile) Write(p []byte) (int, error) {
	rf.fileMutex.Lock()
	defer rf.fileMutex.Unlock()

	// Check if rotation is needed
	if rf.fileSize+int64(len(p)) > rf.maxSize {
		// Rotate the file
		if err := rf.rotate(); err != nil {
			// If rotation fails, still try to write
			n, _ := rf.file.Write(p)
			rf.fileSize += int64(n)
			return n, nil
		}
	}

	n, err := rf.file.Write(p)
	rf.fileSize += int64(n)
	return n, err
}

func (rf *rotatedFile) rotate() error {
	if err := rf.file.Close(); err != nil {
		return err
	}

	// Create backup filename with timestamp
	timestamp := time.Now().Format("20060102_150405")
	dir := filepath.Dir(rf.filePath)
	name := filepath.Base(rf.filePath)
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	backupPath := filepath.Join(dir, fmt.Sprintf("%s.%s%s", base, timestamp, ext))

	// Rename current file to backup
	if err := os.Rename(rf.filePath, backupPath); err != nil {
		return err
	}

	// Open new file
	f, err := os.OpenFile(rf.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	rf.file = f
	rf.fileSize = 0
	return nil
}

func (rf *rotatedFile) Close() error {
	rf.fileMutex.Lock()
	defer rf.fileMutex.Unlock()
	return rf.file.Close()
}

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
	logFiles = make(map[string]io.WriteCloser)
}

// Init initializes the logger with specified level and directory
func Init(levelStr, dir string, maxFileSizeMB int) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	logDir = dir
	maxLogSize = int64(maxFileSizeMB) * 1024 * 1024

	// Parse level
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
	errorLogFile, err := os.OpenFile(filepath.Join(filepath.Clean(dir), "error.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	// Wrap with rotation
	errorRotated := &rotatedFile{
		file:     errorLogFile,
		filePath: filepath.Join(filepath.Clean(dir), "error.log"),
		maxSize:  maxLogSize,
	}
	// Get initial file size
	if info, err := errorLogFile.Stat(); err == nil {
		errorRotated.fileSize = info.Size()
	}
	logFiles["error"] = errorRotated

	// Trade Log
	tradeLogFile, err := os.OpenFile(filepath.Join(filepath.Clean(dir), "trade.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	// Wrap with rotation
	tradeRotated := &rotatedFile{
		file:     tradeLogFile,
		filePath: filepath.Join(filepath.Clean(dir), "trade.log"),
		maxSize:  maxLogSize,
	}
	// Get initial file size
	if info, err := tradeLogFile.Stat(); err == nil {
		tradeRotated.fileSize = info.Size()
	}
	logFiles["trade"] = tradeRotated

	// Custom handler for simple text format without JSON
	Log = slog.New(&plainTextHandler{w: errorRotated, level: logLevel})
	Trade = slog.New(&plainTextHandler{w: tradeRotated, level: logLevel})

	return nil
}

// Get returns a logger with module context
func Get(module string) *slog.Logger {
	if Log == nil {
		return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}
	return Log.With("module", module)
}

// GetTrade returns a trade logger with module context
func GetTrade(module string) *slog.Logger {
	if Trade == nil {
		return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}
	return Trade.With("module", module)
}

// Debug logs a debug message
func Debug(msg string, args ...any) {
	if Log != nil {
		Log.Debug(msg, args...)
	}
}

// Info logs an info message
func Info(msg string, args ...any) {
	if Log != nil {
		Log.Info(msg, args...)
	}
}

// Warn logs a warning message
func Warn(msg string, args ...any) {
	if Log != nil {
		Log.Warn(msg, args...)
	}
}

// Error logs an error message
func Error(msg string, args ...any) {
	if Log != nil {
		Log.Error(msg, args...)
	}
}

// TradeInfo logs a trade info message
func TradeInfo(msg string, args ...any) {
	if Trade != nil {
		Trade.Info(msg, args...)
	}
}

// TradeWarn logs a trade warning message
func TradeWarn(msg string, args ...any) {
	if Trade != nil {
		Trade.Warn(msg, args...)
	}
}

// TradeError logs a trade error message
func TradeError(msg string, args ...any) {
	if Trade != nil {
		Trade.Error(msg, args...)
	}
}

// Close closes all open log files
func Close() error {
	fileMutex.Lock()
	defer fileMutex.Unlock()

	var lastErr error
	for name, f := range logFiles {
		if err := f.Close(); err != nil {
			lastErr = err
		}
		delete(logFiles, name)
	}
	return lastErr
}

// GetLevel returns the current log level
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
