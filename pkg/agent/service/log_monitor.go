package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/pika-monitor/pika/internal/protocol"
)

func (a *Agent) handleLogMonitorConfig(data json.RawMessage) {
	var config protocol.LogMonitorConfig
	if err := json.Unmarshal(data, &config); err != nil {
		slog.Warn("解析日志配置失败", "error", err)
		return
	}
	result := protocol.LogMonitorResult{Revision: config.Revision, Success: true, Message: "日志监控配置已应用"}
	if err := a.logMonitor.Apply(config); err != nil {
		result.Success = false
		result.Message = err.Error()
	}
	_ = a.sendOutboundMessage(protocol.OutboundMessage{Type: protocol.MessageTypeLogMonitorResult, Data: result})
}

func (a *Agent) logEventLoop(ctx context.Context) {
	queued := map[string]uint64{}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pending := a.logMonitor.Pending()
			present := map[string]bool{}
			for _, event := range pending {
				present[event.ID] = true
				if seq, ok := queued[event.ID]; ok && a.outbox.hasSeq(seq) {
					continue
				}
				inFlight, _, _ := a.outbox.stats()
				if inFlight >= outboxMaxEntries/2 {
					continue
				}
				queued[event.ID] = a.outbox.enqueue(protocol.OutboundMessage{Type: protocol.MessageTypeLogEvent, Data: event})
			}
			for id := range queued {
				if !present[id] {
					delete(queued, id)
				}
			}
		}
	}
}
