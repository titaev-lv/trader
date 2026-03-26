// Package main - точка входа в приложение trader
// trader - это сервис для мониторинга и торговли на криптобиржах
// Может работать в режиме монитора (сбор данных), трейдера (исполнение стратегий) или обоих одновременно
package main

import (
	// "encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"trader/internal/config"
	"trader/internal/logger"
	"trader/internal/manager"
)

// Version - текущая версия приложения
// Используется для логирования при старте
const (
	Version = "2.0.2"
)

// main - основная функция приложения
// Порядок инициализации критичен:
// 1. Загрузить конфигурацию (нужна для всех компонентов)
// 2. Инициализировать логирование (нужно для отладки всего остального)
// 3. Инициализировать и запустить менеджер задач
// 4. Обработать сигналы OS для корректного завершения
func main() {
	// Парсируем флаги командной строки
	// Использование: trader -c path/to/config.yaml
	configFile := flag.String("c", "conf/config.yaml", "Path to configuration file")
	flag.Parse()

	// 1. ЗАГРУЗКА КОНФИГУРАЦИИ
	// Конфигурация хранится в YAML файле с параметрами для всех компонентов:
	// - databases: параметры хранилищ/движков
	// - log: уровень логирования, папка для логов
	// - trade: параметры торговли (интервал обновления и т.д.)
	cfg, err := config.Load(*configFile)
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// 2. ИНИЦИАЛИЗАЦИЯ ЛОГИРОВАНИЯ
	// Логирование - самый важный компонент для отладки проблем в production
	// Создает логи в папке (по умолчанию ./logs) и ротирует файлы по размеру
	// Поддерживает разные уровни: debug, info, warn, error
	if err := logger.Init(
		cfg.Logging.Level,
		cfg.Logging.Format,
		cfg.Logging.MaxFileSizeMB,
		cfg.Logging.MaxBackups,
		cfg.Logging.MaxAgeDays,
		cfg.Logging.Compress,
		cfg.Logging.ErrorPath,
		cfg.Logging.OutRequestPath,
		cfg.Logging.WSInPath,
		cfg.Logging.WSOutPath,
		cfg.Logging.AuditPath,
		cfg.Logging.OutRequestToStdout,
		cfg.Logging.WSInToStdout,
		cfg.Logging.WSOutToStdout,
		cfg.Logging.AuditToStdout,
	); err != nil {
		fmt.Printf("Failed to init logger: %+v\n", err)
		os.Exit(1)
	}
	defer logger.Close()

	// Получаем логгер для main модуля
	// Все логи будут помечены префиксом "main" для удобства поиска
	log := logger.Get("main")
	log.Info("\n\n")
	log.Info("==========================================================")
	log.Info("INIT START trader", "version", Version)
	log.Info("Starting trader", "config", *configFile)

	// 3. ИНИЦИАЛИЗАЦИЯ МЕНЕДЖЕРА
	// Менеджер - это сердце приложения
	// Отвечает за:
	// - Получение задач и управление жизненным циклом через CTS-Core
	// - Управление подписками на WebSocket потоки
	// - Запуск/остановку Monitor и Trader компонентов
	mgr := manager.New(cfg)

	// 4. ЗАПУСК МЕНЕДЖЕРА
	// Trader работает как outbound-клиент (WS/REST к CTS-Core и биржам)
	// и не поднимает локальный HTTP сервер для входящих команд.
	if err := mgr.Start(); err != nil {
		log.Error("Failed to start manager", "error", err)
		os.Exit(1)
	}

	// 5. ОБРАБОТКА СИГНАЛОВ ОС
	// Создаем канал для получения сигналов от операционной системы
	// Поддерживаем:
	// - SIGINT (Ctrl+C) - мягкое завершение
	// - SIGTERM (kill -15) - мягкое завершение
	// Это позволяет корректно выключить демон и сохранить состояние
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Ждем сигнала завершения
	sig := <-sigChan
	log.Info("Received signal, shutting down...", "signal", sig)

	// 6. КОРРЕКТНОЕ ЗАВЕРШЕНИЕ
	// Выполняем graceful shutdown - останавливаем все компоненты в правильном порядке
	// 1. Останавливаем менеджер (прекращает мониторинг/торговлю)
	// 2. Закрываем логирование (в defer уже)
	// Это гарантирует, что все данные сохранены и соединения закрыты
	if err := mgr.Stop(); err != nil {
		log.Error("Error during shutdown", "error", err)
	}
	log.Info("Shutdown complete")

	// jsonData, _ := json.MarshalIndent(cfg, "", "  ")
	// fmt.Println(string(jsonData))
}
