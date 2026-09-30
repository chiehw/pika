package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pika-monitor/pika/internal/protocol"
	"github.com/pika-monitor/pika/pkg/agent/config"
	"github.com/pika-monitor/pika/pkg/agent/id"
	"github.com/pika-monitor/pika/pkg/agent/logmonitor"
)

func TestAgentAppliesLogConfigAndDurableAckOverWebSocket(t *testing.T) {
	dir := t.TempDir()
	logfile := filepath.Join(dir, "Clash Party.log")
	os.WriteFile(logfile, nil, 0600)
	applied := make(chan bool, 1)
	received := make(chan protocol.LogEvent, 1)
	configuration := protocol.LogMonitorConfig{Enabled: true, Revision: "version-one", PollIntervalSeconds: 1, Rules: []protocol.LogMonitorRule{{ID: "r", Name: "Clash Party", Enabled: true, Paths: []string{logfile}, Regex: "(?i)error|only", Level: "warning"}}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		var register protocol.InputMessage
		if err = conn.ReadJSON(&register); err != nil {
			return
		}
		conn.WriteJSON(protocol.OutboundMessage{Type: protocol.MessageTypeRegisterAck, Data: protocol.RegisterResponse{AgentID: "test-agent", Status: "success", Reliable: true}})
		conn.WriteJSON(protocol.OutboundMessage{Type: protocol.MessageTypeLogMonitorConfig, Data: configuration})
		for {
			var msg protocol.InputMessage
			if err = conn.ReadJSON(&msg); err != nil {
				return
			}
			if msg.Type == protocol.MessageTypeLogMonitorResult {
				var result protocol.LogMonitorResult
				json.Unmarshal(msg.Data, &result)
				applied <- result.Success
			}
			if msg.Type == protocol.MessageTypeLogEvent {
				var event protocol.LogEvent
				json.Unmarshal(msg.Data, &event)
				conn.WriteJSON(protocol.OutboundMessage{Type: protocol.MessageTypeLogEventAck, Data: protocol.LogEventAck{ID: event.ID}})
				received <- event
			}
			conn.WriteJSON(protocol.OutboundMessage{Type: protocol.MessageTypeAck, Data: protocol.AckData{Seq: msg.Seq}})
		}
	}))
	defer server.Close()
	cfg := config.DefaultConfig()
	cfg.Path = filepath.Join(dir, "agent.yaml")
	cfg.Server.Endpoint = server.URL
	a := New(cfg)
	a.idMgr = id.NewManagerWithPath(filepath.Join(dir, "agent.id"))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go a.logMonitor.Run(ctx)
	go a.logEventLoop(ctx)
	done := make(chan error, 1)
	go func() { done <- a.runOnce(ctx, func() {}) }()
	select {
	case success := <-applied:
		if !success {
			t.Fatal("config rejected")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("config not applied")
	}
	os.WriteFile(logfile, []byte("ERROR native Pika\n"), 0600)
	select {
	case event := <-received:
		if event.Message != "ERROR native Pika" || event.File != logfile {
			t.Fatalf("invalid event: %+v", event)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("log event not sent")
	}
	for i := 0; i < 50 && len(a.logMonitor.Pending()) > 0; i++ {
		time.Sleep(20 * time.Millisecond)
	}
	if len(a.logMonitor.Pending()) > 0 {
		t.Fatal("event ACK did not trim durable queue")
	}
	restored, err := logmonitor.New(filepath.Join(dir, "log-monitor-state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(restored.Pending()) != 0 {
		t.Fatal("ACK was not persisted")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("agent did not stop")
	}
}
