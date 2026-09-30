package protocol

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	MessageTypeLogMonitorConfig MessageType = "log_monitor_config"
	MessageTypeLogMonitorResult MessageType = "log_monitor_result"
	MessageTypeLogEvent         MessageType = "log_event"
	MessageTypeLogEventAck      MessageType = "log_event_ack"
)

type LogMonitorRule struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Enabled         bool     `json:"enabled"`
	Paths           []string `json:"paths"`
	Regex           string   `json:"regex"`
	Level           string   `json:"level"`
	CooldownSeconds int      `json:"cooldownSeconds"`
}

type LogMonitorConfig struct {
	Enabled             bool             `json:"enabled"`
	PollIntervalSeconds int              `json:"pollIntervalSeconds"`
	Revision            string           `json:"revision"`
	Rules               []LogMonitorRule `json:"rules"`
}

func (c *LogMonitorConfig) Validate() error {
	if c.PollIntervalSeconds == 0 {
		c.PollIntervalSeconds = 5
	}
	if c.PollIntervalSeconds < 1 || c.PollIntervalSeconds > 60 {
		return fmt.Errorf("日志检查间隔必须为 1–60 秒")
	}
	if len(c.Rules) > 32 {
		return fmt.Errorf("最多配置 32 条日志规则")
	}
	ids := map[string]bool{}
	for i := range c.Rules {
		r := &c.Rules[i]
		if r.ID == "" || len(r.ID) > 128 || ids[r.ID] {
			return fmt.Errorf("日志规则 ID 为空或重复")
		}
		ids[r.ID] = true
		if strings.TrimSpace(r.Name) == "" || len(r.Name) > 256 {
			return fmt.Errorf("日志规则名称不能为空且不超过 256 字节")
		}
		if len(r.Paths) == 0 || len(r.Paths) > 16 {
			return fmt.Errorf("每条规则需要 1–16 个日志路径")
		}
		for _, p := range r.Paths {
			if strings.TrimSpace(p) == "" || len(p) > 4096 {
				return fmt.Errorf("日志路径为空或过长")
			}
		}
		if r.Regex == "" || len(r.Regex) > 2048 {
			return fmt.Errorf("匹配表达式为空或过长")
		}
		if _, err := regexp.Compile(r.Regex); err != nil {
			return fmt.Errorf("规则 %s 的正则无效: %w", r.Name, err)
		}
		if r.Level == "" {
			r.Level = "warning"
		}
		if r.Level != "info" && r.Level != "warning" && r.Level != "critical" {
			return fmt.Errorf("日志告警级别无效")
		}
		if r.CooldownSeconds < 0 || r.CooldownSeconds > 86400 {
			return fmt.Errorf("通知冷却时间必须为 0–86400 秒")
		}
	}
	return nil
}

type LogMonitorResult struct {
	Revision string `json:"revision"`
	Success  bool   `json:"success"`
	Message  string `json:"message"`
}

type LogEvent struct {
	ID        string `json:"id"`
	RuleID    string `json:"ruleId"`
	RuleName  string `json:"ruleName"`
	File      string `json:"file"`
	Message   string `json:"message"`
	Level     string `json:"level"`
	Timestamp int64  `json:"timestamp"`
}

type LogEventAck struct {
	ID string `json:"id"`
}
