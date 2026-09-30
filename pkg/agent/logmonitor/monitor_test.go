package logmonitor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pika-monitor/pika/internal/protocol"
)

func testConfig(dir string) protocol.LogMonitorConfig {
	return protocol.LogMonitorConfig{Enabled: true, PollIntervalSeconds: 1, Rules: []protocol.LogMonitorRule{{ID: "clash", Name: "Clash Party", Enabled: true, Paths: []string{filepath.Join(dir, "*.log")}, Regex: "(?i)error|only", Level: "warning"}}}
}
func appendLog(t *testing.T, path, text string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err = f.WriteString(text); err != nil {
		t.Fatal(err)
	}
}
func scan(t *testing.T, m *Monitor, now time.Time) {
	t.Helper()
	if err := m.Scan(now); err != nil {
		t.Fatal(err)
	}
}
func ackAll(t *testing.T, m *Monitor) {
	t.Helper()
	for _, e := range m.Pending() {
		if err := m.Acknowledge(e.ID); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAppendPartialRepeatedRotationAndRestart(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Application Support")
	os.MkdirAll(dir, 0700)
	path := filepath.Join(dir, "core.log")
	os.WriteFile(path, []byte("ERROR historical\n"), 0600)
	state := filepath.Join(t.TempDir(), "state.json")
	m, err := New(state)
	if err != nil {
		t.Fatal(err)
	}
	config := testConfig(dir)
	if err = m.Apply(config); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	scan(t, m, now)
	if len(m.Pending()) != 0 {
		t.Fatal("replayed old content")
	}
	appendLog(t, path, "ERROR repeat\nERROR repeat\nOnly partial")
	scan(t, m, now.Add(time.Second))
	events := m.Pending()
	if len(events) != 2 {
		t.Fatalf("repeated lines lost: %v", events)
	}
	restarted, err := New(state)
	if err != nil {
		t.Fatal(err)
	}
	if len(restarted.Pending()) != 2 {
		t.Fatal("pending events did not survive restart")
	}
	ackAll(t, restarted)
	appendLog(t, path, " completed\nINFO normal\n")
	scan(t, restarted, now.Add(2*time.Second))
	events = restarted.Pending()
	if len(events) != 1 || events[0].Message != "Only partial completed" {
		t.Fatalf("partial line lost: %v", events)
	}
	ackAll(t, restarted)
	if err = os.Rename(path, path+".old"); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(path, []byte("ERROR rotated first line\n"), 0600)
	scan(t, restarted, now.Add(3*time.Second))
	if len(restarted.Pending()) != 1 {
		t.Fatal("rotated first line lost")
	}
	ackAll(t, restarted)
	os.WriteFile(path, []byte("ERROR short\n"), 0600)
	scan(t, restarted, now.Add(4*time.Second))
	if len(restarted.Pending()) != 1 {
		t.Fatal("truncated file lost")
	}
	ackAll(t, restarted)
	fresh := filepath.Join(dir, "next-day.log")
	os.WriteFile(fresh, []byte("ERROR new file\n"), 0600)
	scan(t, restarted, now.Add(5*time.Second))
	if len(restarted.Pending()) != 1 {
		t.Fatal("new file first line lost")
	}
	if err = restarted.Apply(config); err != nil {
		t.Fatal(err)
	}
	scan(t, restarted, now.Add(6*time.Second))
	if len(restarted.Pending()) != 1 {
		t.Fatal("reconnect replayed file")
	}
}

func TestCooldownLongLineAndInvalidConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	os.WriteFile(path, nil, 0600)
	m, err := New(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	c := testConfig(dir)
	c.Rules[0].CooldownSeconds = 10
	if err = m.Apply(c); err != nil {
		t.Fatal(err)
	}
	bad := testConfig(dir)
	bad.Rules[0].Regex = "["
	if err = m.Apply(bad); err == nil {
		t.Fatal("invalid config accepted")
	}
	appendLog(t, path, strings.Repeat("x", maxLineBytes+5)+"\nERROR one\nERROR two\n")
	now := time.Now()
	scan(t, m, now)
	if len(m.Pending()) != 1 {
		t.Fatalf("cooldown failed: %v", m.Pending())
	}
	ackAll(t, m)
	appendLog(t, path, "ERROR three\n")
	scan(t, m, now.Add(11*time.Second))
	if len(m.Pending()) != 1 {
		t.Fatal("cooldown never expired")
	}
	info, err := os.Stat(m.path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("state file permissions: %v", info.Mode())
	}
}

func TestFullPendingQueueBackpressuresFileReading(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	os.WriteFile(path, nil, 0600)
	m, _ := New(filepath.Join(t.TempDir(), "state.json"))
	if err := m.Apply(testConfig(dir)); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < maxPending; i++ {
		key := fmt.Sprintf("pending-%d", i)
		m.state.Pending[key] = protocol.LogEvent{ID: key}
	}
	appendLog(t, path, "ERROR retained\n")
	now := time.Now()
	scan(t, m, now)
	if m.state.Cursors["clash\x00"+path].Offset != 0 {
		t.Fatal("queue overflow consumed unread logs")
	}
	if err := m.Acknowledge("pending-0"); err != nil {
		t.Fatal(err)
	}
	scan(t, m, now.Add(time.Second))
	found := false
	for _, event := range m.Pending() {
		if event.Message == "ERROR retained" {
			found = true
		}
	}
	if !found {
		t.Fatal("backpressured line never delivered")
	}
}

func TestDisabledPeriodsAreNotBackfilled(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	os.WriteFile(path, nil, 0600)
	m, _ := New(filepath.Join(t.TempDir(), "state.json"))
	c := testConfig(dir)
	if err := m.Apply(c); err != nil {
		t.Fatal(err)
	}
	c.Enabled = false
	if err := m.Apply(c); err != nil {
		t.Fatal(err)
	}
	appendLog(t, path, "ERROR while disabled\n")
	c.Enabled = true
	if err := m.Apply(c); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	scan(t, m, now)
	if len(m.Pending()) != 0 {
		t.Fatal("global disabled period was backfilled")
	}
	c.Rules[0].Enabled = false
	if err := m.Apply(c); err != nil {
		t.Fatal(err)
	}
	appendLog(t, path, "ERROR while rule disabled\n")
	c.Rules[0].Enabled = true
	if err := m.Apply(c); err != nil {
		t.Fatal(err)
	}
	scan(t, m, now.Add(time.Second))
	if len(m.Pending()) != 0 {
		t.Fatal("disabled rule period was backfilled")
	}
	appendLog(t, path, "ERROR after enabling\n")
	scan(t, m, now.Add(2*time.Second))
	if len(m.Pending()) != 1 {
		t.Fatal("new occurrence was missed")
	}
}
