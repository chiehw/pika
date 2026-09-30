package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/pika-monitor/pika/internal/models"
	"github.com/pika-monitor/pika/internal/protocol"
	"go.uber.org/zap"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func TestLogEventUsesExistingNotificationTemplateAndDeduplicates(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "pika.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AutoMigrate(&models.Agent{}, &models.AlertRule{}, &models.AlertRecord{}, &models.AlertState{}, &models.Property{}); err != nil {
		t.Fatal(err)
	}
	received := make(chan map[string]string, 10)
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		received <- payload
		w.WriteHeader(200)
	}))
	defer sink.Close()
	channels := []models.NotificationChannelConfig{{Type: "webhook", Enabled: true, Config: map[string]interface{}{"url": sink.URL, "customBody": `{"body":"{{alert.message}}","rule":"{{alert.logRuleName}}","file":"{{alert.logFile}}","host":"{{agent.name}}"}`}}}
	data, _ := json.Marshal(channels)
	if err = db.Create(&models.Property{ID: PropertyIDNotificationChannels, Value: string(data)}).Error; err != nil {
		t.Fatal(err)
	}
	config := protocol.LogMonitorConfig{Enabled: true, PollIntervalSeconds: 5, Rules: []protocol.LogMonitorRule{{ID: "r1", Name: "Clash Party", Enabled: true, Paths: []string{"/tmp/*.log"}, Regex: "error", Level: "warning"}}}
	agent := models.Agent{ID: "mac", Name: "macbook-air", Enabled: true, LogMonitorConfig: datatypes.NewJSONType(models.LogMonitorConfigData{LogMonitorConfig: config})}
	if err = db.Create(&agent).Error; err != nil {
		t.Fatal(err)
	}
	rule := models.AlertRule{ID: "notify", Name: "现有通知规则", Enabled: true, TargetType: models.AlertRuleTargetAll, Channels: datatypes.JSONSlice[string]{"webhook"}, Notifications: datatypes.NewJSONType(models.AlertNotifications{LogEnabled: true})}
	if err = db.Create(&rule).Error; err != nil {
		t.Fatal(err)
	}
	logger := zap.NewNop()
	properties := NewPropertyService(logger, db)
	rules := NewAlertRuleService(logger, db, properties)
	alerts := NewAlertService(logger, db, properties, rules, nil, NewNotifier(logger))
	defer alerts.Shutdown()
	service := NewLogMonitorService(logger, db, nil, alerts)
	event := protocol.LogEvent{ID: "event-one", RuleID: "r1", Message: "error \"quoted\"\nnext", File: "/tmp/core.log", Timestamp: time.Now().UnixMilli()}
	for i := 0; i < 2; i++ {
		if err = service.HandleEvent(context.Background(), agent.ID, event); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case body := <-received:
		if body["body"] != event.Message || body["rule"] != "Clash Party" || body["file"] != event.File || body["host"] != agent.Name {
			t.Fatalf("template changed: %v", body)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no notification")
	}
	var count int64
	db.Model(&models.AlertRecord{}).Count(&count)
	if count != 1 {
		t.Fatalf("replay created %d records", count)
	}
	// Identical content with a new event identity is a new occurrence.
	event.ID = "event-two"
	if err = service.HandleEvent(context.Background(), agent.ID, event); err != nil {
		t.Fatal(err)
	}
	select {
	case <-received:
	case <-time.After(5 * time.Second):
		t.Fatal("repeated text was swallowed")
	}
	// Turning off log notifications keeps the event record without sending it.
	rule.Notifications = datatypes.NewJSONType(models.AlertNotifications{})
	db.Save(&rule)
	event.ID = "event-three"
	if err = service.HandleEvent(context.Background(), agent.ID, event); err != nil {
		t.Fatal(err)
	}
	var record models.AlertRecord
	db.First(&record, "log_event_id = ?", agent.ID+":"+event.ID)
	if record.NotificationStatus != "skipped" {
		t.Fatalf("notification switch ignored: %v", record)
	}
}
