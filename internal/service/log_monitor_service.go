package service

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/google/uuid"
	"github.com/pika-monitor/pika/internal/models"
	"github.com/pika-monitor/pika/internal/protocol"
	"github.com/pika-monitor/pika/internal/repo"
	"github.com/pika-monitor/pika/internal/websocket"
	"go.uber.org/zap"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type LogMonitorService struct {
	db     *gorm.DB
	agents *repo.AgentRepo
	ws     *websocket.Manager
	alerts *AlertService
	logger *zap.Logger
}

func NewLogMonitorService(logger *zap.Logger, db *gorm.DB, ws *websocket.Manager, alerts *AlertService) *LogMonitorService {
	return &LogMonitorService{db: db, agents: repo.NewAgentRepo(db), ws: ws, alerts: alerts, logger: logger}
}
func (s *LogMonitorService) GetConfig(ctx context.Context, agentID string) (*models.LogMonitorConfigData, error) {
	agent, err := s.agents.FindById(ctx, agentID)
	if err != nil {
		return nil, err
	}
	c := agent.LogMonitorConfig.Data()
	if c.PollIntervalSeconds == 0 {
		c.PollIntervalSeconds = 5
	}
	if c.Rules == nil {
		c.Rules = []protocol.LogMonitorRule{}
	}
	return &c, nil
}
func (s *LogMonitorService) UpdateConfig(ctx context.Context, agentID string, req protocol.LogMonitorConfig) error {
	if err := req.Validate(); err != nil {
		return err
	}
	agent, err := s.agents.FindById(ctx, agentID)
	if err != nil {
		return err
	}
	req.Revision = uuid.NewString()
	c := models.LogMonitorConfigData{LogMonitorConfig: req, ApplyStatus: "pending"}
	if err = s.db.WithContext(ctx).Model(&models.Agent{}).Where("id = ?", agentID).Update("log_monitor_config", datatypes.NewJSONType(c)).Error; err != nil {
		return err
	}
	if !agent.Enabled {
		req.Enabled = false
	}
	message, err := json.Marshal(protocol.OutboundMessage{Type: protocol.MessageTypeLogMonitorConfig, Data: req})
	if err != nil {
		return err
	}
	// Offline probes receive the stored config on their next registration.
	if err = s.ws.SendToClient(agentID, message); err != nil {
		s.logger.Info("日志配置已保存，等待探针上线", zap.String("agentId", agentID))
	}
	return nil
}
func (s *LogMonitorService) InitialConfig(ctx context.Context, agentID string) (protocol.LogMonitorConfig, error) {
	agent, err := s.agents.FindById(ctx, agentID)
	if err != nil {
		return protocol.LogMonitorConfig{}, err
	}
	c := agent.LogMonitorConfig.Data().LogMonitorConfig
	if !agent.Enabled {
		c.Enabled = false
	}
	if c.PollIntervalSeconds == 0 {
		c.PollIntervalSeconds = 5
	}
	return c, nil
}
func (s *LogMonitorService) SendConfig(ctx context.Context, agentID string) error {
	config, err := s.InitialConfig(ctx, agentID)
	if err != nil {
		return err
	}
	data, err := json.Marshal(protocol.OutboundMessage{Type: protocol.MessageTypeLogMonitorConfig, Data: config})
	if err != nil {
		return err
	}
	return s.ws.SendToClient(agentID, data)
}
func (s *LogMonitorService) HandleResult(ctx context.Context, agentID string, result protocol.LogMonitorResult) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var agent models.Agent
		if err := tx.First(&agent, "id = ?", agentID).Error; err != nil {
			return err
		}
		c := agent.LogMonitorConfig.Data()
		if c.Revision != result.Revision {
			return nil
		}
		c.ApplyStatus = "success"
		if !result.Success {
			c.ApplyStatus = "failed"
		}
		c.ApplyMessage = result.Message
		return tx.Model(&models.Agent{}).Where("id = ?", agentID).Update("log_monitor_config", datatypes.NewJSONType(c)).Error
	})
}
func (s *LogMonitorService) HandleEvent(ctx context.Context, agentID string, event protocol.LogEvent) error {
	if event.ID == "" || len(event.ID) > 128 || len(event.Message) > 64*1024 || len(event.File) > 4096 {
		return fmt.Errorf("日志事件字段无效")
	}
	agent, err := s.agents.FindById(ctx, agentID)
	if err != nil {
		return err
	}
	if !agent.Enabled {
		return nil
	}
	c := agent.LogMonitorConfig.Data()
	if !c.Enabled {
		return nil
	}
	var rule *protocol.LogMonitorRule
	for i := range c.Rules {
		if c.Rules[i].ID == event.RuleID && c.Rules[i].Enabled {
			rule = &c.Rules[i]
			break
		}
	}
	if rule == nil {
		return nil
	}
	pattern, err := regexp.Compile(rule.Regex)
	if err != nil {
		return err
	}
	if !pattern.MatchString(event.Message) {
		return nil
	}
	event.RuleName = rule.Name
	event.Level = rule.Level
	return s.alerts.RecordLogEvent(ctx, &agent, event)
}
