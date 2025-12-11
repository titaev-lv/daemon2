// Package api предоставляет REST API для управления демоном
// Позволяет внешним приложениям (например фронтенд, мониторинг) контролировать состояние демона
package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"ctdaemon/internal/config"
	"ctdaemon/internal/logger"
	"ctdaemon/internal/manager"
)

// Server - HTTP API сервер для управления демоном
// Предоставляет endpoints для:
// - Получения статуса (запущен ли, как долго работает)
// - Управления (старт, остановка)
// - Получения информации о версии
type Server struct {
	// cfg - конфигурация сервера (порт и т.д.)
	cfg config.ServerConfig
	// mgr - менеджер приложения который управляет компонентами
	mgr *manager.Manager
	// version - версия приложения для отправки в ответе
	version string
}

// New - создает новый API сервер
// cfg - конфигурация (содержит Port)
// mgr - менеджер приложения
// version - строка версии приложения
func New(cfg config.ServerConfig, mgr *manager.Manager, version string) *Server {
	return &Server{
		cfg:     cfg,
		mgr:     mgr,
		version: version,
	}
}

// Start - запускает HTTP сервер и слушает входящие запросы
// Блокирует текущий goroutine, поэтому вызывается в отдельной goroutine из main.go
// Использует http.ListenAndServe которая никогда не возвращается (пока не будет ошибка)
func (s *Server) Start() error {
	// Регистрируем обработчики для разных путей
	mux := http.NewServeMux()
	mux.HandleFunc("/control", s.handleControl)
	mux.HandleFunc("/status", s.handleStatus)
	mux.HandleFunc("/version", s.handleVersion)

	// Формируем адрес для слушания (:8080, :9000 и т.д.)
	addr := fmt.Sprintf(":%d", s.cfg.Port)
	logger.Get("api").Info("API server listening", "addr", addr)
	// Это блокирует навсегда пока не будет ошибка или shutdown
	return http.ListenAndServe(addr, mux)
}

// handleControl - обработчик для управления демоном (старт/остановка)
// Поддерживаемые запросы:
// - PUT /control?action=start - запустить демон
// - PUT /control?action=stop - остановить демон
func (s *Server) handleControl(w http.ResponseWriter, r *http.Request) {
	// Разрешаем только PUT метод (не GET, POST и т.д.)
	if r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Получаем параметр action из query string (?action=start)
	action := r.URL.Query().Get("action")
	if action == "" {
		http.Error(w, "Missing action parameter", http.StatusBadRequest)
		return
	}

	var err error
	var statusMsg string

	// Выполняем нужное действие
	switch action {
	case "start":
		err = s.mgr.Start()
		if err == manager.ErrAlreadyRunning {
			statusMsg = "already started"
		} else if err == nil {
			statusMsg = "started"
		}
	case "stop":
		err = s.mgr.Stop()
		if err == manager.ErrNotRunning {
			statusMsg = "already stopped"
		} else if err == nil {
			statusMsg = "stopped"
		}
	default:
		http.Error(w, "Invalid action", http.StatusBadRequest)
		return
	}

	// Если была ошибка которая не является "уже запущен/остановлен" - отправляем 500
	if err != nil && err != manager.ErrAlreadyRunning && err != manager.ErrNotRunning {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Отправляем успешный ответ в JSON
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": statusMsg})
}

// handleStatus - обработчик для получения статуса демона
// Поддерживаемый запрос:
// - GET /status - вернуть JSON с информацией о статусе (running, uptime, start_time и т.д.)
// Эта информация используется фронтенд приложением для отображения статуса демона
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	// Разрешаем только GET метод
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// Получаем статус от менеджера (он возвращает map[string]interface{})
	status := s.mgr.Status()
	// Отправляем статус в JSON формате
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// handleVersion - обработчик для получения версии приложения
// Поддерживаемый запрос:
// - GET /version - вернуть JSON с версией (например "2.0.1")
// Используется для проверки какая версия запущена (полезно при обновлениях)
func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	// Разрешаем только GET метод
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// Отправляем версию в JSON
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"version": s.version})
}
