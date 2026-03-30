package manager

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"trader/internal/config"
	"trader/internal/core/ctscorews"
	"trader/internal/core/ws"
	"trader/internal/logger"
	"trader/internal/state"
	"trader/internal/task"
)

var (
	ErrAlreadyRunning         = errors.New("system is already running")
	ErrNotRunning             = errors.New("system is not running")
	ErrTaskUpdateNotSupported = errors.New("task source does not support updates")
)

type Manager struct {
	cfg       *config.Config
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	mu        sync.RWMutex
	isRunning bool

	startTime    time.Time
	shutdownTime time.Time
	shutdownErr  error

	loopSource   task.Source
	loopSubMgr   *task.SubscriptionManager
	coreWSClient *ctscorews.Client

	snapMu    sync.RWMutex
	snapshot  RuntimeSnapshot
	iteration int64
}

const (
	GracefulShutdownTimeout = 30 * time.Second
)

type RuntimeSnapshot struct {
	Running         bool
	Iteration       int64
	LastTrigger     string
	LastSyncAt      *time.Time
	LastDiffAt      *time.Time
	LastApplyAt     *time.Time
	LastSuccessAt   *time.Time
	LastError       string
	LastErrorStage  string
	LastSubscribe   int
	LastUnsubscribe int
}

func New(cfg *config.Config) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	stateMgr := state.GetInstance()
	logger.Get("manager").Info("State manager initialized", "is_running", stateMgr.IsRunning())

	wsPool := ws.NewPool()
	loopSubMgr := task.NewSubscriptionManager(wsPool)
	loopSource := task.NewStaticSource()

	m := &Manager{
		cfg:        cfg,
		ctx:        ctx,
		cancel:     cancel,
		loopSource: loopSource,
		loopSubMgr: loopSubMgr,
		snapshot: RuntimeSnapshot{
			Running: false,
		},
	}

	if cfg.CoreConnections.WS.Enabled && cfg.CoreConnections.WS.URL != "" {
		m.coreWSClient = ctscorews.New(cfg.CoreConnections.WS, func(raw []byte) error {
			return m.ApplyUpdateEnvelope(raw)
		})
	}

	return m
}

func (m *Manager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.isRunning {
		logger.Get("manager").Info("System already running")
		return ErrAlreadyRunning
	}

	m.ctx, m.cancel = context.WithCancel(context.Background())
	m.isRunning = true
	m.startTime = time.Now()
	m.shutdownErr = nil

	logger.Get("manager").Info("Starting runtime loop", "mode", "event-only")

	if err := state.GetInstance().SetRunning(true); err != nil {
		logger.Get("manager").Error("Failed to persist running state", "error", err)
	}

	m.setSnapshot(func(s *RuntimeSnapshot) {
		s.Running = true
		s.LastError = ""
		s.LastErrorStage = ""
	})

	m.wg.Add(1)
	go m.runLoop()

	if m.coreWSClient != nil {
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			m.coreWSClient.Run(m.ctx)
		}()
		logger.Get("manager").Info("CTS-Core WS client starting", "url", m.cfg.CoreConnections.WS.URL)
	}

	logger.Get("manager").Info("Runtime loop started")
	return nil
}

func (m *Manager) Stop() error {
	m.mu.RLock()
	isRunning := m.isRunning
	m.mu.RUnlock()

	if !isRunning {
		logger.Get("manager").Info("System not running, skipping shutdown")
		return ErrNotRunning
	}

	return m.doStop()
}

func (m *Manager) doStop() error {
	m.mu.Lock()
	if !m.isRunning {
		m.mu.Unlock()
		logger.Get("manager").Info("System not running, skipping shutdown")
		return ErrNotRunning
	}
	m.isRunning = false
	logger.Get("manager").Info("Initiating graceful shutdown...", "timeout", GracefulShutdownTimeout)
	m.shutdownTime = time.Now()
	cancel := m.cancel
	m.mu.Unlock()

	if err := state.GetInstance().SetRunning(false); err != nil {
		logger.Get("manager").Error("Failed to persist stopped state", "error", err)
	}

	if cancel != nil {
		cancel()
	}

	waitCtx, waitCancel := context.WithTimeout(context.Background(), GracefulShutdownTimeout)
	defer waitCancel()

	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		logger.Get("manager").Info("Graceful shutdown completed successfully")
		m.mu.Lock()
		m.shutdownErr = nil
		m.mu.Unlock()
	case <-waitCtx.Done():
		logger.Get("manager").Error("Shutdown timeout", "error", waitCtx.Err())
		m.mu.Lock()
		m.shutdownErr = waitCtx.Err()
		m.mu.Unlock()
	}

	m.setSnapshot(func(s *RuntimeSnapshot) {
		s.Running = false
	})

	m.mu.RLock()
	err := m.shutdownErr
	m.mu.RUnlock()
	return err
}

func (m *Manager) runLoop() {
	defer m.wg.Done()

	log := logger.Get("manager.loop")
	log.Debug("runtime loop started")

	watchCh := m.loopSource.Watch(m.ctx)

	for {
		select {
		case <-m.ctx.Done():
			log.Info("runtime loop stopping")
			return
		case _, ok := <-watchCh:
			if !ok {
				watchCh = nil
				continue
			}
			m.runIteration("event")
		}
	}
}

func (m *Manager) runIteration(trigger string) {
	log := logger.Get("manager.loop")

	m.iteration++
	iteration := m.iteration
	m.setSnapshot(func(s *RuntimeSnapshot) {
		s.Iteration = iteration
		s.LastTrigger = trigger
	})

	log.Debug("sync started", "iteration", iteration, "trigger", trigger)
	tasksData, err := m.loopSource.GetTasks(m.ctx)
	syncAt := time.Now().UTC()
	if err != nil {
		m.recordLoopError("sync", err, iteration)
		log.Error("sync failed", "iteration", iteration, "trigger", trigger, "error", err)
		return
	}
	m.setSnapshot(func(s *RuntimeSnapshot) {
		s.LastSyncAt = &syncAt
	})

	log.Debug("diff started", "iteration", iteration, "trigger", trigger)
	diff, err := m.loopSubMgr.Merge(tasksData)
	diffAt := time.Now().UTC()
	if err != nil {
		m.recordLoopError("diff", err, iteration)
		log.Error("diff failed", "iteration", iteration, "trigger", trigger, "error", err)
		return
	}
	m.setSnapshot(func(s *RuntimeSnapshot) {
		s.LastDiffAt = &diffAt
	})

	log.Debug("apply started", "iteration", iteration, "trigger", trigger, "to_subscribe", len(diff.ToSubscribe), "to_unsubscribe", len(diff.Unsubscribe))
	if err := m.loopSubMgr.ApplyDiff(diff); err != nil {
		m.recordLoopError("apply", err, iteration)
		log.Error("apply failed", "iteration", iteration, "trigger", trigger, "error", err)
		return
	}

	applyAt := time.Now().UTC()
	m.setSnapshot(func(s *RuntimeSnapshot) {
		s.LastApplyAt = &applyAt
		s.LastSuccessAt = &applyAt
		s.LastError = ""
		s.LastErrorStage = ""
		s.LastSubscribe = len(diff.ToSubscribe)
		s.LastUnsubscribe = len(diff.Unsubscribe)
	})

	log.Debug("iteration complete", "iteration", iteration, "trigger", trigger, "to_subscribe", len(diff.ToSubscribe), "to_unsubscribe", len(diff.Unsubscribe))
}

func (m *Manager) recordLoopError(stage string, err error, iteration int64) {
	m.setSnapshot(func(s *RuntimeSnapshot) {
		s.Iteration = iteration
		s.LastError = err.Error()
		s.LastErrorStage = stage
	})
}

func (m *Manager) setSnapshot(update func(*RuntimeSnapshot)) {
	m.snapMu.Lock()
	defer m.snapMu.Unlock()
	update(&m.snapshot)
}

func (m *Manager) Status() map[string]interface{} {
	m.mu.RLock()
	isRunning := m.isRunning
	startTime := m.startTime
	shutdownTime := m.shutdownTime
	shutdownErr := m.shutdownErr
	m.mu.RUnlock()

	m.snapMu.RLock()
	snapshot := cloneRuntimeSnapshot(m.snapshot)
	m.snapMu.RUnlock()

	uptime := time.Duration(0)
	if isRunning {
		uptime = time.Since(startTime)
	} else if !startTime.IsZero() {
		uptime = shutdownTime.Sub(startTime)
	}

	totalSeconds := int64(uptime.Seconds())
	days := totalSeconds / 86400
	hours := (totalSeconds % 86400) / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60

	var uptimeStr string
	if days > 0 {
		uptimeStr = fmt.Sprintf("%dd %dh %dm %ds", days, hours, minutes, seconds)
	} else if hours > 0 {
		uptimeStr = fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
	} else if minutes > 0 {
		uptimeStr = fmt.Sprintf("%dm %ds", minutes, seconds)
	} else {
		uptimeStr = fmt.Sprintf("%ds", seconds)
	}

	return map[string]interface{}{
		"running": isRunning,
		"uptime":  uptimeStr,
		"start_time": func() interface{} {
			if startTime.IsZero() {
				return nil
			}
			return startTime
		}(),
		"shutdown_time": func() interface{} {
			if shutdownTime.IsZero() {
				return nil
			}
			return shutdownTime
		}(),
		"error": shutdownErr,
		"runtime": map[string]interface{}{
			"iteration":        snapshot.Iteration,
			"last_trigger":     snapshot.LastTrigger,
			"last_sync_at":     timeValueOrNil(snapshot.LastSyncAt),
			"last_diff_at":     timeValueOrNil(snapshot.LastDiffAt),
			"last_apply_at":    timeValueOrNil(snapshot.LastApplyAt),
			"last_success_at":  timeValueOrNil(snapshot.LastSuccessAt),
			"last_error":       snapshot.LastError,
			"last_error_stage": snapshot.LastErrorStage,
			"to_subscribe":     snapshot.LastSubscribe,
			"to_unsubscribe":   snapshot.LastUnsubscribe,
			"core_ws_reconnect": func() interface{} {
				if m.coreWSClient == nil {
					return nil
				}
				metrics := m.coreWSClient.ReconnectMetrics()
				return map[string]interface{}{
					"total":              metrics.Total,
					"by_reason":          metrics.ByReason,
					"close_4009_seq_gap": metrics.Close4009SeqGap,
				}
			}(),
			"core_ws_ping": func() interface{} {
				if m.coreWSClient == nil {
					return nil
				}
				stats := m.coreWSClient.PingStats()
				return map[string]interface{}{
					"last_ping_at": stats.LastPingAt,
					"last_pong_at": stats.LastPongAt,
					"last_rtt_ms": func() interface{} {
						if stats.LastRTT == 0 {
							return nil
						}
						return float64(stats.LastRTT) / float64(time.Millisecond)
					}(),
				}
			}(),
		},
	}
}

func (m *Manager) IsRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.isRunning
}

func (m *Manager) GetContext() context.Context {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.ctx
}

func (m *Manager) RuntimeSnapshot() RuntimeSnapshot {
	m.snapMu.RLock()
	defer m.snapMu.RUnlock()
	return cloneRuntimeSnapshot(m.snapshot)
}

func cloneRuntimeSnapshot(src RuntimeSnapshot) RuntimeSnapshot {
	out := src
	out.LastSyncAt = cloneTimePtr(src.LastSyncAt)
	out.LastDiffAt = cloneTimePtr(src.LastDiffAt)
	out.LastApplyAt = cloneTimePtr(src.LastApplyAt)
	out.LastSuccessAt = cloneTimePtr(src.LastSuccessAt)
	return out
}

func cloneTimePtr(src *time.Time) *time.Time {
	if src == nil {
		return nil
	}
	v := *src
	return &v
}

func timeValueOrNil(src *time.Time) interface{} {
	if src == nil {
		return nil
	}
	return *src
}

func (m *Manager) UpdateTasks(data *task.TasksData) error {
	if data == nil {
		return nil
	}

	updater, ok := m.loopSource.(interface{ SetTasks(*task.TasksData) })
	if !ok {
		return ErrTaskUpdateNotSupported
	}

	updater.SetTasks(data)
	logger.Get("manager").Info(
		"tasks updated",
		"monitoring_tasks", len(data.MonitoringTasks),
		"trading_tasks", len(data.TradingTasks),
	)

	return nil
}

func (m *Manager) ApplyUpdateCommand(cmd task.UpdateCommand) error {
	norm, err := cmd.Normalized()
	if err != nil {
		return err
	}

	if err := m.UpdateTasks(norm.Data); err != nil {
		return err
	}

	logger.Get("manager").Info(
		"task update command applied",
		"mode", norm.Mode,
		"source", norm.Source,
		"request_id", norm.RequestID,
		"monitoring_tasks", len(norm.Data.MonitoringTasks),
		"trading_tasks", len(norm.Data.TradingTasks),
	)

	return nil
}

func (m *Manager) ApplyUpdateEnvelope(raw []byte) error {
	base, err := m.loopSource.GetTasks(m.ctx)
	if err != nil {
		return err
	}

	cmd, err := task.DecodeUpdateCommandFromJSONWithBase(raw, base)
	if err != nil {
		return err
	}
	return m.ApplyUpdateCommand(cmd)
}

func (m *Manager) Shutdown() error {
	return m.Stop()
}
