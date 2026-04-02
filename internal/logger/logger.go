package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	Log            *slog.Logger
	OutRequestLog  *slog.Logger
	WSCoreLog      *slog.Logger
	WSExchangesLog *slog.Logger
	AuditLog       *slog.Logger
	Trade          *slog.Logger
	logLevel       slog.Level
	logDir         string
	logFiles       map[string]io.WriteCloser
	fileMutex      sync.RWMutex

	wsExchangeLogs          map[string]*slog.Logger
	wsExchangeDir           string
	wsExchangeFormat        string
	wsExchangeMaxSizeMB     int
	wsExchangeMaxBackups    int
	wsExchangeMaxAgeDays    int
	wsExchangeCompress      bool
	wsExchangeToStdout      bool
	wsExchangesEnabled      bool
	wsExchangeSingleEnabled bool
)

func init() {
	logFiles = make(map[string]io.WriteCloser)
	wsExchangeLogs = make(map[string]*slog.Logger)
}

func Init(levelStr, format string, maxFileSizeMB int, maxBackups int, maxAgeDays int, compress bool, errorLogPath, outRequestLogPath, wsCoreLogPath, wsExchangesLogPath, auditLogPath string, outRequestToStdout, wsCoreToStdout, wsExchangesToStdout, wsExchangesLogEnable, wsExchangeSingleLogEnable, auditToStdout bool) error {
	paths := []string{errorLogPath, outRequestLogPath, wsCoreLogPath, auditLogPath}
	if wsExchangesLogEnable || wsExchangeSingleLogEnable {
		paths = append(paths, wsExchangesLogPath)
	}
	if err := validateLogPaths(paths...); err != nil {
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

	fileMutex.Lock()
	for name, f := range logFiles {
		_ = f.Close()
		delete(logFiles, name)
	}
	wsExchangeLogs = make(map[string]*slog.Logger)
	fileMutex.Unlock()

	if err := ensureLogFileExists(errorLogPath); err != nil {
		return err
	}
	errorLogFile, err := newRotatingLogFile(errorLogPath, maxFileSizeMB, maxBackups, maxAgeDays, compress)
	if err != nil {
		return err
	}
	logFiles["error"] = errorLogFile

	if err := ensureLogFileExists(outRequestLogPath); err != nil {
		return err
	}
	outRequestLogFile, err := newRotatingLogFile(outRequestLogPath, maxFileSizeMB, maxBackups, maxAgeDays, compress)
	if err != nil {
		return err
	}
	logFiles["out_request"] = outRequestLogFile

	if err := ensureLogFileExists(wsCoreLogPath); err != nil {
		return err
	}
	wsCoreLogFile, err := newRotatingLogFile(wsCoreLogPath, maxFileSizeMB, maxBackups, maxAgeDays, compress)
	if err != nil {
		return err
	}
	logFiles["ws_core"] = wsCoreLogFile

	var wsExchangesLogFile *lumberjack.Logger
	if wsExchangesLogEnable {
		if err := ensureLogFileExists(wsExchangesLogPath); err != nil {
			return err
		}
		wsExchangesLogFile, err = newRotatingLogFile(wsExchangesLogPath, maxFileSizeMB, maxBackups, maxAgeDays, compress)
		if err != nil {
			return err
		}
		logFiles["ws_exchanges"] = wsExchangesLogFile
	}

	if err := ensureLogFileExists(auditLogPath); err != nil {
		return err
	}
	auditLogFile, err := newRotatingLogFile(auditLogPath, maxFileSizeMB, maxBackups, maxAgeDays, compress)
	if err != nil {
		return err
	}
	logFiles["audit"] = auditLogFile

	wsExchangeDir = filepath.Dir(wsExchangesLogPath)
	wsExchangeFormat = format
	wsExchangeMaxSizeMB = maxFileSizeMB
	wsExchangeMaxBackups = maxBackups
	wsExchangeMaxAgeDays = maxAgeDays
	wsExchangeCompress = compress
	wsExchangeToStdout = wsExchangesToStdout
	wsExchangesEnabled = wsExchangesLogEnable
	wsExchangeSingleEnabled = wsExchangeSingleLogEnable

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
	wsCoreWriter := io.Writer(wsCoreLogFile)
	if wsCoreToStdout {
		wsCoreWriter = io.MultiWriter(os.Stdout, wsCoreLogFile)
	}
	wsExchangesWriter := io.Writer(io.Discard)
	if wsExchangesLogEnable {
		wsExchangesWriter = io.Writer(wsExchangesLogFile)
	}
	if wsExchangesLogEnable && wsExchangesToStdout {
		wsExchangesWriter = io.MultiWriter(os.Stdout, wsExchangesLogFile)
	}
	auditWriter := io.Writer(auditLogFile)
	if auditToStdout {
		auditWriter = io.MultiWriter(os.Stdout, auditLogFile)
	}

	Log = slog.New(newHandler(format, errorWriter, opts))
	OutRequestLog = slog.New(newHandler(format, outRequestWriter, opts))
	WSCoreLog = slog.New(newHandler(format, wsCoreWriter, opts))
	WSExchangesLog = slog.New(newHandler(format, wsExchangesWriter, opts))
	AuditLog = slog.New(newHandler(format, auditWriter, auditOpts))
	Trade = Log
	slog.SetDefault(Log)

	return nil
}

func newHandler(format string, writer io.Writer, opts *slog.HandlerOptions) slog.Handler {
	if strings.EqualFold(format, "text") {
		return slog.NewTextHandler(&textValueOnlyWriter{dst: writer}, opts)
	}
	return slog.NewJSONHandler(writer, opts)
}

type textValueOnlyWriter struct {
	dst io.Writer
}

func (w *textValueOnlyWriter) Write(p []byte) (int, error) {
	if w == nil || w.dst == nil {
		return len(p), nil
	}

	text := string(p)
	lines := strings.SplitAfter(text, "\n")
	var b strings.Builder
	b.Grow(len(text))
	for _, line := range lines {
		if line == "" {
			continue
		}
		hasNewline := strings.HasSuffix(line, "\n")
		trimmed := strings.TrimSuffix(line, "\n")
		b.WriteString(stripTextPrefixKeys(trimmed))
		if hasNewline {
			b.WriteByte('\n')
		}
	}

	if _, err := w.dst.Write([]byte(b.String())); err != nil {
		return 0, err
	}
	return len(p), nil
}

func stripTextPrefixKeys(line string) string {
	if !strings.HasPrefix(line, "time=") {
		return line
	}

	idx := len("time=")
	timeVal, next, ok := readAttrValue(line, idx)
	if !ok {
		return line
	}

	idx = next
	levelVal := ""
	if strings.HasPrefix(line[idx:], "level=") {
		idx += len("level=")
		var levelOK bool
		levelVal, idx, levelOK = readAttrValue(line, idx)
		if !levelOK {
			return line
		}
	}

	if !strings.HasPrefix(line[idx:], "msg=") {
		return line
	}
	idx += len("msg=")
	msgVal, idx, ok := readAttrValue(line, idx)
	if !ok {
		return line
	}
	msgVal = normalizeMsgValue(msgVal)

	rest := strings.TrimLeft(line[idx:], " ")
	if levelVal != "" {
		if rest != "" {
			return timeVal + " " + levelVal + " " + msgVal + " " + rest
		}
		return timeVal + " " + levelVal + " " + msgVal
	}

	if rest != "" {
		return timeVal + " " + msgVal + " " + rest
	}
	return timeVal + " " + msgVal
}

func normalizeMsgValue(raw string) string {
	if len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"' {
		unquoted, err := strconv.Unquote(raw)
		if err == nil {
			return unquoted
		}
		return strings.Trim(raw, "\"")
	}
	return raw
}

func readAttrValue(line string, start int) (value string, next int, ok bool) {
	if start >= len(line) {
		return "", start, false
	}

	if line[start] == '"' {
		i := start + 1
		escaped := false
		end := -1
		for i < len(line) {
			ch := line[i]
			if escaped {
				escaped = false
				i++
				continue
			}
			if ch == '\\' {
				escaped = true
				i++
				continue
			}
			if ch == '"' {
				end = i + 1
				i = end
				for i < len(line) && line[i] == ' ' {
					i++
				}
				return line[start:end], i, true
			}
			i++
		}
		return "", start, false
	}

	i := start
	for i < len(line) && line[i] != ' ' {
		i++
	}
	val := line[start:i]
	for i < len(line) && line[i] == ' ' {
		i++
	}
	return val, i, true
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

func GetWSCore(_ string) *slog.Logger {
	if WSCoreLog == nil {
		if Log == nil {
			return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo, ReplaceAttr: replaceTimeAttr}))
		}
		return Log
	}
	return WSCoreLog
}

func GetWSExchanges(module string) *slog.Logger {
	if WSExchangesLog == nil {
		return Get(module)
	}
	return WSExchangesLog.With("module", module)
}

func GetWSExchange(exchangeID, module string) *slog.Logger {
	normalized := normalizeExchangeID(exchangeID)
	if normalized == "" {
		return GetWSExchanges(module)
	}
	if !wsExchangeSingleEnabled {
		return GetWSExchanges(module).With("exchange_id", normalized)
	}
	if module == "" {
		module = "ws_exchange"
	}

	fileMutex.RLock()
	existing := wsExchangeLogs[normalized]
	fileMutex.RUnlock()
	if existing != nil {
		return existing.With("module", module, "exchange_id", normalized)
	}

	fileMutex.Lock()
	defer fileMutex.Unlock()

	existing = wsExchangeLogs[normalized]
	if existing != nil {
		return existing.With("module", module, "exchange_id", normalized)
	}

	if wsExchangeDir == "" {
		if logDir == "" {
			return GetWSExchanges(module).With("exchange_id", normalized)
		}
		wsExchangeDir = logDir
	}

	path := filepath.Join(wsExchangeDir, fmt.Sprintf("ws-%s.log", normalized))
	if err := ensureLogFileExists(path); err != nil {
		if Log != nil {
			Log.Warn("failed to create exchange ws log file, fallback to ws_exchanges", "exchange_id", normalized, "path", path, "error", err)
		}
		return GetWSExchanges(module).With("exchange_id", normalized)
	}

	rotator, err := newRotatingLogFile(path, wsExchangeMaxSizeMB, wsExchangeMaxBackups, wsExchangeMaxAgeDays, wsExchangeCompress)
	if err != nil {
		if Log != nil {
			Log.Warn("failed to init exchange ws logger, fallback to ws_exchanges", "exchange_id", normalized, "path", path, "error", err)
		}
		return GetWSExchanges(module).With("exchange_id", normalized)
	}

	writer := io.Writer(rotator)
	if wsExchangeToStdout {
		writer = io.MultiWriter(os.Stdout, rotator)
	}

	base := slog.New(newHandler(wsExchangeFormat, writer, &slog.HandlerOptions{Level: logLevel, ReplaceAttr: replaceTimeAttr}))
	wsExchangeLogs[normalized] = base
	logFiles["ws_exchange:"+normalized] = rotator

	return base.With("module", module, "exchange_id", normalized)
}

// GetAudit возвращает аудит-логгер.
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
	for name, f := range logFiles {
		if err := f.Close(); err != nil {
			lastErr = err
		}
		delete(logFiles, name)
	}
	wsExchangeLogs = make(map[string]*slog.Logger)
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

func newRotatingLogFile(path string, maxSize, maxBackups, maxAge int, compress bool) (*lumberjack.Logger, error) {
	l := &lumberjack.Logger{
		Filename:   path,
		MaxSize:    maxSize,
		MaxBackups: maxBackups,
		MaxAge:     maxAge,
		Compress:   compress,
	}

	if shouldRotateOnStartup(path) {
		if err := l.Rotate(); err != nil {
			return nil, fmt.Errorf("rotate log on startup %s: %w", path, err)
		}
	}

	return l, nil
}

func shouldRotateOnStartup(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	if !info.Mode().IsRegular() {
		return false
	}
	return info.Size() > 0
}

func normalizeExchangeID(exchangeID string) string {
	trimmed := strings.ToLower(strings.TrimSpace(exchangeID))
	if trimmed == "" {
		return ""
	}

	var b strings.Builder
	b.Grow(len(trimmed))
	for i := 0; i < len(trimmed); i++ {
		ch := trimmed[i]
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_' {
			b.WriteByte(ch)
			continue
		}
		b.WriteByte('-')
	}

	normalized := strings.Trim(b.String(), "-_")
	if normalized == "" {
		return ""
	}

	for strings.Contains(normalized, "--") {
		normalized = strings.ReplaceAll(normalized, "--", "-")
	}

	return normalized
}
