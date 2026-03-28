package task

import (
	"errors"
	"time"

	"trader/internal/core/exchange"
)

var (
	ErrUnsupportedUpdateMode = errors.New("unsupported update mode")
	ErrReplacePayloadNil     = errors.New("replace update requires tasks payload")
)

type UpdateMode string

const (
	UpdateModeReplace UpdateMode = "replace"
	UpdateModeReset   UpdateMode = "reset"
)

// UpdateCommand описывает внутреннюю команду изменения набора задач.
// Следующий WS-слой должен конвертировать входящие task.* сообщения в этот формат.
type UpdateCommand struct {
	Mode      UpdateMode
	Data      *TasksData
	Source    string
	RequestID string
}

func (c UpdateCommand) Normalized() (UpdateCommand, error) {
	mode := c.Mode
	if mode == "" {
		mode = UpdateModeReplace
	}

	norm := c
	norm.Mode = mode

	switch mode {
	case UpdateModeReplace:
		if c.Data == nil {
			return UpdateCommand{}, ErrReplacePayloadNil
		}
		norm.Data = &TasksData{
			Timestamp:       normalizeTimestamp(c.Data.Timestamp),
			MonitoringTasks: cloneMonitoringTasks(c.Data.MonitoringTasks),
			TradingTasks:    cloneTradingTasks(c.Data.TradingTasks),
		}
		return norm, nil
	case UpdateModeReset:
		norm.Data = &TasksData{
			Timestamp:       time.Now().UTC().Unix(),
			MonitoringTasks: []*exchange.MonitoringTask{},
			TradingTasks:    []*exchange.TradingTask{},
		}
		return norm, nil
	default:
		return UpdateCommand{}, ErrUnsupportedUpdateMode
	}
}

func normalizeTimestamp(ts int64) int64 {
	if ts > 0 {
		return ts
	}
	return time.Now().UTC().Unix()
}
