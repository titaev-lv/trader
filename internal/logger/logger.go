package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	Log           *slog.Logger
	OutRequestLog *slog.Logger
	WSInLog       *slog.Logger
	WSOutLog      *slog.Logger
	AuditLog      *slog.Logger
	Trade         *slog.Logger
	logLevel      slog.Level
	logDir        string
	logFiles      map[string]io.WriteCloser
	fileMutex     sync.RWMutex
)

func init() {
	logFiles = make(map[string]io.WriteCloser)
}

func Init(levelStr, format string, maxFileSizeMB int, maxBackups int, maxAgeDays int, compress bool, errorLogPath, outRequestLogPath, wsInLogPath, wsOutLogPath, auditLogPath string, outRequestToStdout, wsInToStdout, wsOutToStdout, auditToStdout bool) error {
	if err := validateLogPaths(errorLogPath, outRequestLogPath, wsInLogPath, wsOutLogPath, auditLogPath); err != nil {
		return err
	}
	if format == "" {
		format = "json"
	}

	logDir = filepath.Dir(errorLogPath)
	if maxFileSizeMB <= 0 {
		maxFileSizeMB = 100
	}
	if maxBackups <= 0 {
		maxBackups = 10
	}
	if maxAgeDays <= 0 {
		maxAgeDays = 30
	}

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

	if err := ensureLogFileExists(errorLogPath); err != nil {
		return err
	}
	errorLogFile := &lumberjack.Logger{
		Filename:   errorLogPath,
		MaxSize:    maxFileSizeMB,
		MaxBackups: maxBackups,
		MaxAge:     maxAgeDays,
		Compress:   compress,
	}
	logFiles["error"] = errorLogFile

	if err := ensureLogFileExists(outRequestLogPath); err != nil {
		return err
	}
	outRequestLogFile := &lumberjack.Logger{
		Filename:   outRequestLogPath,
		MaxSize:    maxFileSizeMB,
		MaxBackups: maxBackups,
		MaxAge:     maxAgeDays,
		Compress:   compress,
	}
	logFiles["out_request"] = outRequestLogFile

	if err := ensureLogFileExists(wsInLogPath); err != nil {
		return err
	}
	wsInLogFile := &lumberjack.Logger{
		Filename:   wsInLogPath,
		MaxSize:    maxFileSizeMB,
		MaxBackups: maxBackups,
		MaxAge:     maxAgeDays,
		Compress:   compress,
	}
	logFiles["ws_in"] = wsInLogFile

	if err := ensureLogFileExists(wsOutLogPath); err != nil {
		return err
	}
	wsOutLogFile := &lumberjack.Logger{
		Filename:   wsOutLogPath,
		MaxSize:    maxFileSizeMB,
		MaxBackups: maxBackups,
		MaxAge:     maxAgeDays,
		Compress:   compress,
	}
	logFiles["ws_out"] = wsOutLogFile

	if err := ensureLogFileExists(auditLogPath); err != nil {
		return err
	}
	auditLogFile := &lumberjack.Logger{
		Filename:   auditLogPath,
		MaxSize:    maxFileSizeMB,
		MaxBackups: maxBackups,
		MaxAge:     maxAgeDays,
		Compress:   compress,
	}
	logFiles["audit"] = auditLogFile

	opts := &slog.HandlerOptions{
		Level:       logLevel,
		ReplaceAttr: replaceTimeAttr,
	}
	auditOpts := &slog.HandlerOptions{
		Level:       logLevel,
		ReplaceAttr: replaceAuditAttr,
	}

	errorWriter := io.MultiWriter(os.Stdout, errorLogFile)
	outRequestWriter := io.Writer(outRequestLogFile)
	if outRequestToStdout {
		outRequestWriter = io.MultiWriter(os.Stdout, outRequestLogFile)
	}
	wsInWriter := io.Writer(wsInLogFile)
	if wsInToStdout {
		wsInWriter = io.MultiWriter(os.Stdout, wsInLogFile)
	}
	wsOutWriter := io.Writer(wsOutLogFile)
	if wsOutToStdout {
		wsOutWriter = io.MultiWriter(os.Stdout, wsOutLogFile)
	}
	auditWriter := io.Writer(auditLogFile)
	if auditToStdout {
		auditWriter = io.MultiWriter(os.Stdout, auditLogFile)
	}

	Log = slog.New(newHandler(format, errorWriter, opts))
	OutRequestLog = slog.New(newHandler(format, outRequestWriter, opts))
	WSInLog = slog.New(newHandler(format, wsInWriter, opts))
	WSOutLog = slog.New(newHandler(format, wsOutWriter, opts))
	AuditLog = slog.New(newHandler(format, auditWriter, auditOpts))
	Trade = Log
	slog.SetDefault(Log)

	return nil
}

func newHandler(format string, writer io.Writer, opts *slog.HandlerOptions) slog.Handler {
	if strings.EqualFold(format, "text") {
		return slog.NewTextHandler(writer, opts)
	}
	return slog.NewJSONHandler(writer, opts)
}

// Get - возвращает логгер для конкретного модуля
// module: имя модуля (main, db, trade, orderbook и т.д.)
// Используется для идентификации источника логов: "2023-12-11 15:04:05 [INFO] [db] Connection established"
func Get(module string) *slog.Logger {
	if Log == nil {
		return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo, ReplaceAttr: replaceTimeAttr}))
	}
	return Log.With("module", module)
}

func GetOutRequest(module string) *slog.Logger {
	if OutRequestLog == nil {
		return Get(module)
	}
	return OutRequestLog.With("module", module)
}

func GetWSIn(module string) *slog.Logger {
	if WSInLog == nil {
		return Get(module)
	}
	return WSInLog.With("module", module)
}

func GetWSOut(module string) *slog.Logger {
	if WSOutLog == nil {
		return Get(module)
	}
	return WSOutLog.With("module", module)
}

func GetAudit(module string) *slog.Logger {
	if AuditLog == nil {
		return Get(module)
	}
	return AuditLog.With("module", module)
}

// GetTrade - возвращает торговый логгер с контекстом модуля
func GetTrade(module string) *slog.Logger {
	if Trade == nil {
		return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo, ReplaceAttr: replaceTimeAttr}))
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

// TradeInfo - логирует информацию о торговле в основной поток ошибок
// Торговые события пишутся в unified stream (error.log + stdout)
// Пример: "Opened position BTC/USDT, entry price 45000"
func TradeInfo(msg string, args ...any) {
	if Trade != nil {
		Trade.Info(msg, args...)
	}
}

// TradeWarn - логирует предупреждение о торговле в основной поток ошибок
// Используется для проблем в торговле которые могут повлиять на результат
// Пример: "Position margin approaching liquidation level"
func TradeWarn(msg string, args ...any) {
	if Trade != nil {
		Trade.Warn(msg, args...)
	}
}

// TradeError - логирует критичную ошибку о торговле в основной поток ошибок
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

func replaceTimeAttr(_ []string, attr slog.Attr) slog.Attr {
	if attr.Key != slog.TimeKey {
		return attr
	}
	if t, ok := attr.Value.Any().(time.Time); ok {
		attr.Value = slog.StringValue(t.UTC().Format("2006-01-02T15:04:05.000000Z"))
	}
	return attr
}

func replaceAuditAttr(groups []string, attr slog.Attr) slog.Attr {
	attr = replaceTimeAttr(groups, attr)
	if attr.Key == slog.LevelKey {
		return slog.Attr{}
	}
	return attr
}

func validateLogPaths(paths ...string) error {
	checked := make(map[string]struct{})
	for _, path := range paths {
		if path == "" {
			return fmt.Errorf("log file path is empty")
		}
		dir := filepath.Dir(path)
		if dir == "" || dir == "." {
			return fmt.Errorf("invalid log file path %s", path)
		}
		if _, exists := checked[dir]; exists {
			continue
		}
		checked[dir] = struct{}{}
		if err := os.MkdirAll(dir, 0750); err != nil {
			return fmt.Errorf("create log dir %s: %w", dir, err)
		}

		file, err := os.CreateTemp(dir, ".write-test-*")
		if err != nil {
			return fmt.Errorf("create write test in %s: %w", dir, err)
		}
		name := file.Name()
		if _, err := file.WriteString("test"); err != nil {
			_ = file.Close()
			_ = os.Remove(name)
			return fmt.Errorf("write test in %s: %w", dir, err)
		}
		if err := file.Close(); err != nil {
			_ = os.Remove(name)
			return fmt.Errorf("close write test in %s: %w", dir, err)
		}

		rotated := name + ".rotate"
		if err := os.Rename(name, rotated); err != nil {
			_ = os.Remove(name)
			return fmt.Errorf("rename write test in %s: %w", dir, err)
		}
		if err := os.Remove(rotated); err != nil {
			return fmt.Errorf("cleanup write test in %s: %w", dir, err)
		}
	}

	return nil
}

func ensureLogFileExists(path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0640)
	if err != nil {
		return fmt.Errorf("create log file %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close log file %s: %w", path, err)
	}
	return nil
}
