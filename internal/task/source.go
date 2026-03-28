package task

import (
	"context"
	"sync"
	"time"

	"trader/internal/core/exchange"
)

// Source возвращает актуальный снимок задач для runtime loop.
type Source interface {
	GetTasks(ctx context.Context) (*TasksData, error)
	Watch(ctx context.Context) <-chan struct{}
}

// StaticSource - in-memory источник задач для базового runtime wiring.
type StaticSource struct {
	mu       sync.RWMutex
	task     *TasksData
	watchers map[int]chan struct{}
	nextID   int
}

func NewStaticSource() *StaticSource {
	return &StaticSource{
		task: &TasksData{
			Timestamp:       time.Now().UTC().Unix(),
			MonitoringTasks: []*exchange.MonitoringTask{},
			TradingTasks:    []*exchange.TradingTask{},
		},
		watchers: make(map[int]chan struct{}),
	}
}

func (s *StaticSource) GetTasks(_ context.Context) (*TasksData, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return &TasksData{
		Timestamp:       time.Now().UTC().Unix(),
		MonitoringTasks: cloneMonitoringTasks(s.task.MonitoringTasks),
		TradingTasks:    cloneTradingTasks(s.task.TradingTasks),
	}, nil
}

func (s *StaticSource) Watch(ctx context.Context) <-chan struct{} {
	ch := make(chan struct{}, 1)

	s.mu.Lock()
	id := s.nextID
	s.nextID++
	s.watchers[id] = ch
	s.mu.Unlock()

	go func() {
		<-ctx.Done()
		s.mu.Lock()
		if existing, ok := s.watchers[id]; ok {
			delete(s.watchers, id)
			close(existing)
		}
		s.mu.Unlock()
	}()

	return ch
}

func (s *StaticSource) SetTasks(data *TasksData) {
	if data == nil {
		return
	}

	s.mu.Lock()
	s.task = &TasksData{
		Timestamp:       data.Timestamp,
		MonitoringTasks: cloneMonitoringTasks(data.MonitoringTasks),
		TradingTasks:    cloneTradingTasks(data.TradingTasks),
	}

	watchers := make([]chan struct{}, 0, len(s.watchers))
	for _, ch := range s.watchers {
		watchers = append(watchers, ch)
	}
	s.mu.Unlock()

	for _, ch := range watchers {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func cloneMonitoringTasks(in []*exchange.MonitoringTask) []*exchange.MonitoringTask {
	if len(in) == 0 {
		return []*exchange.MonitoringTask{}
	}
	out := make([]*exchange.MonitoringTask, 0, len(in))
	for _, item := range in {
		if item == nil {
			continue
		}
		copyItem := *item
		out = append(out, &copyItem)
	}
	return out
}

func cloneTradingTasks(in []*exchange.TradingTask) []*exchange.TradingTask {
	if len(in) == 0 {
		return []*exchange.TradingTask{}
	}
	out := make([]*exchange.TradingTask, 0, len(in))
	for _, item := range in {
		if item == nil {
			continue
		}
		copyItem := *item
		out = append(out, &copyItem)
	}
	return out
}
