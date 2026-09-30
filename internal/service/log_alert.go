package service

import (
	"context"
	"time"

	"github.com/pika-monitor/pika/internal/models"
	"github.com/pika-monitor/pika/internal/protocol"
)

func (s *AlertService) RecordLogEvent(ctx context.Context, agent *models.Agent, event protocol.LogEvent) error {
	now := time.Now().UnixMilli()
	firedAt := event.Timestamp
	if firedAt <= 0 || firedAt > now+60000 {
		firedAt = now
	}
	key := agent.ID + ":" + event.ID
	record := &models.AlertRecord{LogEventID: &key, AgentID: agent.ID, AgentName: agent.Name, AlertType: "log", Message: event.Message, LogRuleName: event.RuleName, LogFile: event.File, Level: event.Level, Status: "notice", FiredAt: firedAt, CreatedAt: now, NotificationStatus: "skipped"}
	config, err := s.alertRuleService.ResolveForAgent(ctx, agent.ID)
	if err != nil {
		return err
	}
	notify := config != nil && config.Notifications.LogEnabled && !config.IsInMaintenance(time.Now())
	if config != nil {
		record.ConfigID = config.ConfigID
		record.ConfigName = config.Name
	}
	if notify {
		record.NotificationStatus = "pending"
	}
	created, err := s.AlertRecordRepo.CreateLogRecord(ctx, record)
	if err != nil {
		return err
	}
	if !created {
		return nil
	}
	if notify {
		return s.notificationQueue.Enqueue(record.ID, agent)
	}
	return nil
}
