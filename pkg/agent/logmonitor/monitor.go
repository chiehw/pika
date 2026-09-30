// Package logmonitor reads local log files and durably queues matching events.
package logmonitor

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pika-monitor/pika/internal/protocol"
)

const (
	maxPending   = 4096
	maxFiles     = 256
	maxLineBytes = 64 * 1024
	maxReadBytes = 1024 * 1024
)

type cursor struct {
	Prefix   []byte `json:"prefix,omitempty"`
	Identity string `json:"identity"`
	Offset   int64  `json:"offset"`
	Skipping bool   `json:"skipping,omitempty"`
}
type diskState struct {
	Config     protocol.LogMonitorConfig    `json:"config"`
	Cursors    map[string]cursor            `json:"cursors"`
	Signatures map[string]string            `json:"signatures"`
	Pending    map[string]protocol.LogEvent `json:"pending"`
	LastEmit   map[string]int64             `json:"lastEmit"`
}
type Monitor struct {
	mu            sync.Mutex
	path          string
	state         diskState
	patterns      map[string]*regexp.Regexp
	nextScan      time.Time
	persistFailed bool
}

func New(path string) (*Monitor, error) {
	m := &Monitor{path: path, patterns: map[string]*regexp.Regexp{}}
	data, err := os.ReadFile(path)
	if err == nil {
		if err = json.Unmarshal(data, &m.state); err != nil {
			return nil, fmt.Errorf("读取日志监控状态: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if m.state.Cursors == nil {
		m.state.Cursors = map[string]cursor{}
	}
	if m.state.Signatures == nil {
		m.state.Signatures = map[string]string{}
	}
	if m.state.Pending == nil {
		m.state.Pending = map[string]protocol.LogEvent{}
	}
	if m.state.LastEmit == nil {
		m.state.LastEmit = map[string]int64{}
	}
	for _, r := range m.state.Config.Rules {
		p, err := regexp.Compile(r.Regex)
		if err != nil {
			return nil, err
		}
		m.patterns[r.ID] = p
	}
	return m, nil
}

func expand(pattern string) (string, error) {
	if strings.HasPrefix(pattern, "~/") || strings.HasPrefix(pattern, "~\\") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		pattern = filepath.Join(home, pattern[2:])
	}
	if !filepath.IsAbs(pattern) {
		return "", fmt.Errorf("日志路径必须为绝对路径: %s", pattern)
	}
	if _, err := filepath.Match(pattern, ""); err != nil {
		return "", fmt.Errorf("日志路径通配符无效: %w", err)
	}
	return pattern, nil
}

func (m *Monitor) Apply(c protocol.LogMonitorConfig) error {
	if err := c.Validate(); err != nil {
		return err
	}
	patterns := map[string]*regexp.Regexp{}
	for i := range c.Rules {
		if !c.Enabled || !c.Rules[i].Enabled {
			patterns[c.Rules[i].ID] = regexp.MustCompile(c.Rules[i].Regex)
			continue
		}
		for j, path := range c.Rules[i].Paths {
			p, err := expand(path)
			if err != nil {
				return err
			}
			c.Rules[i].Paths[j] = p
		}
		patterns[c.Rules[i].ID] = regexp.MustCompile(c.Rules[i].Regex)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	// Prepare a new state before committing: an invalid/inaccessible rule leaves the previous configuration active.
	encoded, _ := json.Marshal(m.state)
	var next diskState
	if err := json.Unmarshal(encoded, &next); err != nil {
		return err
	}
	valid := map[string]bool{}
	enabledRules := map[string]bool{}
	for _, r := range c.Rules {
		valid[r.ID] = true
		enabledRules[r.ID] = r.Enabled
		signatureBytes, _ := json.Marshal(struct {
			Paths   []string
			Regex   string
			Enabled bool
		}{r.Paths, r.Regex, r.Enabled})
		signature := string(signatureBytes)
		if next.Signatures[r.ID] != signature || next.Config.Enabled != c.Enabled {
			for key := range next.Cursors {
				if strings.HasPrefix(key, r.ID+"\x00") {
					delete(next.Cursors, key)
				}
			}
			var files []string
			var err error
			if c.Enabled && r.Enabled {
				files, err = ruleFiles(r)
			}
			if err != nil {
				return err
			}
			for _, path := range files {
				f, err := os.Open(path)
				if err != nil {
					return fmt.Errorf("打开日志 %s: %w", path, err)
				}
				info, err := f.Stat()
				if err != nil {
					f.Close()
					return err
				}
				if !info.Mode().IsRegular() {
					f.Close()
					continue
				}
				id, err := fileIdentity(f)
				prefix, prefixErr := filePrefix(f)
				f.Close()
				if prefixErr != nil {
					return prefixErr
				}
				if err != nil {
					return err
				}
				next.Cursors[r.ID+"\x00"+path] = cursor{Identity: id, Offset: info.Size(), Prefix: prefix}
			}
			next.Signatures[r.ID] = signature
		}
	}
	for id := range next.Signatures {
		if !valid[id] {
			delete(next.Signatures, id)
			delete(next.LastEmit, id)
		}
	}
	for key := range next.Cursors {
		if !valid[strings.SplitN(key, "\x00", 2)[0]] {
			delete(next.Cursors, key)
		}
	}
	for id, event := range next.Pending {
		if !c.Enabled || !enabledRules[event.RuleID] {
			delete(next.Pending, id)
		}
	}
	next.Config = c
	old := m.state
	m.state = next
	if err := m.save(); err != nil {
		m.state = old
		return err
	}
	m.patterns = patterns
	m.nextScan = time.Time{}
	return nil
}

func ruleFiles(r protocol.LogMonitorRule) ([]string, error) {
	seen := map[string]bool{}
	var files []string
	for _, pattern := range r.Paths {
		matched, err := filepath.Glob(pattern)
		if err != nil {
			return nil, err
		}
		for _, path := range matched {
			if !seen[path] {
				seen[path] = true
				files = append(files, path)
				if len(files) > maxFiles {
					return nil, fmt.Errorf("规则 %s 匹配超过 %d 个文件", r.Name, maxFiles)
				}
			}
		}
	}
	return files, nil
}

func (m *Monitor) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := m.Scan(time.Now()); err != nil {
				slog.Warn("日志监控扫描失败", "error", err)
			}
		}
	}
}

// Scan reads complete appended lines. A partial line remains unread until its newline arrives.
func (m *Monitor) Scan(now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.persistFailed {
		if err := m.save(); err != nil {
			return err
		}
	}
	if !m.state.Config.Enabled || now.Before(m.nextScan) {
		return nil
	}
	interval := m.state.Config.PollIntervalSeconds
	if interval < 1 {
		interval = 5
	}
	m.nextScan = now.Add(time.Duration(interval) * time.Second)
	var firstErr error
	for _, r := range m.state.Config.Rules {
		if !r.Enabled {
			continue
		}
		files, err := ruleFiles(r)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		for _, path := range files {
			if len(m.state.Pending) >= maxPending {
				break
			}
			if err := m.readFile(r, path, now); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	if err := m.save(); err != nil {
		return err
	}
	return firstErr
}

func (m *Monitor) readFile(r protocol.LogMonitorRule, path string, now time.Time) error {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	identity, err := fileIdentity(f)
	if err != nil {
		return err
	}
	prefix, err := filePrefix(f)
	if err != nil {
		return err
	}
	key := r.ID + "\x00" + path
	c := m.state.Cursors[key]
	if c.Identity != identity || info.Size() < c.Offset || (len(c.Prefix) > 0 && !bytes.HasPrefix(prefix, c.Prefix)) {
		c = cursor{Identity: identity, Prefix: prefix}
	}
	if len(c.Prefix) == 0 {
		c.Prefix = prefix
	}
	if _, err = f.Seek(c.Offset, io.SeekStart); err != nil {
		return err
	}
	reader := bufio.NewReaderSize(f, maxLineBytes)
	consumed := 0
	for consumed < maxReadBytes && len(m.state.Pending) < maxPending {
		line, readErr := reader.ReadSlice('\n')
		if errors.Is(readErr, bufio.ErrBufferFull) {
			c.Offset += int64(len(line))
			consumed += len(line)
			c.Skipping = true
			continue
		}
		if errors.Is(readErr, io.EOF) {
			if c.Skipping {
				c.Offset += int64(len(line))
			}
			break
		}
		if readErr != nil {
			return readErr
		}
		c.Offset += int64(len(line))
		consumed += len(line)
		if c.Skipping {
			c.Skipping = false
			continue
		}
		message := strings.TrimSuffix(strings.TrimSuffix(string(line), "\n"), "\r")
		if !m.patterns[r.ID].MatchString(message) {
			continue
		}
		if r.CooldownSeconds > 0 && now.UnixMilli()-m.state.LastEmit[r.ID] < int64(r.CooldownSeconds)*1000 {
			continue
		}
		event := protocol.LogEvent{ID: uuid.NewString(), RuleID: r.ID, RuleName: r.Name, File: path, Message: message, Level: r.Level, Timestamp: now.UnixMilli()}
		m.state.Pending[event.ID] = event
		m.state.LastEmit[r.ID] = now.UnixMilli()
	}
	m.state.Cursors[key] = c
	return nil
}

func (m *Monitor) Pending() []protocol.LogEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.persistFailed {
		return nil
	}
	events := make([]protocol.LogEvent, 0, len(m.state.Pending))
	for _, event := range m.state.Pending {
		events = append(events, event)
	}
	sort.Slice(events, func(i, j int) bool {
		if events[i].Timestamp == events[j].Timestamp {
			return events[i].ID < events[j].ID
		}
		return events[i].Timestamp < events[j].Timestamp
	})
	return events
}

func (m *Monitor) Acknowledge(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.state.Pending, id)
	return m.save()
}

func (m *Monitor) save() (err error) {
	defer func() { m.persistFailed = err != nil }()
	if err = os.MkdirAll(filepath.Dir(m.path), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(m.state)
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(m.path), ".log-monitor-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if _, err = temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err = temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err = temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), m.path)
}

func filePrefix(f *os.File) ([]byte, error) {
	prefix := make([]byte, 64)
	n, err := f.ReadAt(prefix, 0)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return prefix[:n], nil
}
