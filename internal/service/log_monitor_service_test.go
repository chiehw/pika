package service

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/pika-monitor/pika/internal/models"
	"github.com/pika-monitor/pika/internal/protocol"
	"github.com/pika-monitor/pika/internal/websocket"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func TestLogConfigurationRevisionAndDisabledAgent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "config.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AutoMigrate(&models.Agent{}); err != nil {
		t.Fatal(err)
	}
	db.Create(&models.Agent{ID: "mac", Enabled: true})
	svc := NewLogMonitorService(zap.NewNop(), db, websocket.NewManager(zap.NewNop()), nil)
	config := protocol.LogMonitorConfig{Enabled: true, Rules: []protocol.LogMonitorRule{{ID: "rule", Name: "日志", Enabled: true, Paths: []string{"/tmp/*.log"}, Regex: "error"}}}
	if err = svc.UpdateConfig(context.Background(), "mac", config); err != nil {
		t.Fatal(err)
	}
	stored, err := svc.GetConfig(context.Background(), "mac")
	if err != nil {
		t.Fatal(err)
	}
	if stored.ApplyStatus != "pending" || stored.Revision == "" || stored.PollIntervalSeconds != 5 {
		t.Fatalf("invalid config: %+v", stored)
	}
	if err = svc.HandleResult(context.Background(), "mac", protocol.LogMonitorResult{Revision: "old", Success: true}); err != nil {
		t.Fatal(err)
	}
	stored, _ = svc.GetConfig(context.Background(), "mac")
	if stored.ApplyStatus != "pending" {
		t.Fatal("stale result overwrote new config")
	}
	if err = svc.HandleResult(context.Background(), "mac", protocol.LogMonitorResult{Revision: stored.Revision, Success: true}); err != nil {
		t.Fatal(err)
	}
	stored, _ = svc.GetConfig(context.Background(), "mac")
	if stored.ApplyStatus != "success" {
		t.Fatal("result was not persisted")
	}
	db.Model(&models.Agent{}).Where("id = ?", "mac").Update("enabled", false)
	initial, err := svc.InitialConfig(context.Background(), "mac")
	if err != nil {
		t.Fatal(err)
	}
	if initial.Enabled {
		t.Fatal("disabled agent kept reading logs")
	}
}
