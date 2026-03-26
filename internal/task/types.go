package task

import "trader/internal/core/exchange"

// TasksData хранит входные задачи подписок независимо от источника.
type TasksData struct {
	Timestamp       int64
	MonitoringTasks []*exchange.MonitoringTask
	TradingTasks    []*exchange.TradingTask
}
