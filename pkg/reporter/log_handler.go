package reporter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gaskaj/OpenAgentFramework/pkg/apitypes"
)

// LogHandler is a slog.Handler that forwards log records to the control plane
// for real-time streaming. It wraps an inner handler so logs are still written
// locally (e.g. to stderr) in addition to being forwarded.
type LogHandler struct {
	inner     slog.Handler
	cfg       Config
	client    *http.Client
	buf       []apitypes.LogEntry
	mu        sync.Mutex
	minLevel  slog.Level
	attrs     []slog.Attr
	groups    []string
	done      chan struct{}
	closed    bool
}

// NewLogHandler creates a handler that tees log output to the control plane.
// Logs at or above minLevel are forwarded; all logs pass through to inner.
func NewLogHandler(inner slog.Handler, cfg Config, minLevel slog.Level) *LogHandler {
	cfg.defaults()
	h := &LogHandler{
		inner:    inner,
		cfg:      cfg,
		client:   &http.Client{Timeout: 5 * time.Second},
		minLevel: minLevel,
		done:     make(chan struct{}),
	}
	go h.flushLoop()
	return h
}

// Enabled reports whether the handler handles records at the given level.
// The inner handler's decision is authoritative; forwarding uses its own threshold.
func (h *LogHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

// Handle processes the record: always passes to inner, and queues for forwarding
// if the level meets the threshold.
func (h *LogHandler) Handle(ctx context.Context, r slog.Record) error {
	// Always write locally
	err := h.inner.Handle(ctx, r)

	// Forward to control plane if at or above threshold
	if r.Level >= h.minLevel {
		fields := make(map[string]any)
		// Include handler-level attrs
		for _, a := range h.attrs {
			fields[a.Key] = a.Value.Any()
		}
		// Include record attrs
		r.Attrs(func(a slog.Attr) bool {
			fields[a.Key] = a.Value.Any()
			return true
		})

		entry := apitypes.LogEntry{
			AgentName: h.cfg.AgentName,
			Level:     r.Level.String(),
			Message:   r.Message,
			Fields:    fields,
			Timestamp: r.Time,
		}

		h.mu.Lock()
		h.buf = append(h.buf, entry)
		shouldFlush := len(h.buf) >= 50
		h.mu.Unlock()

		if shouldFlush {
			go h.flush()
		}
	}

	return err
}

// WithAttrs returns a new handler with the given attributes.
func (h *LogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &LogHandler{
		inner:    h.inner.WithAttrs(attrs),
		cfg:      h.cfg,
		client:   h.client,
		buf:      h.buf,
		minLevel: h.minLevel,
		attrs:    append(h.attrs, attrs...),
		groups:   h.groups,
		done:     h.done,
	}
}

// WithGroup returns a new handler with the given group name.
func (h *LogHandler) WithGroup(name string) slog.Handler {
	return &LogHandler{
		inner:    h.inner.WithGroup(name),
		cfg:      h.cfg,
		client:   h.client,
		buf:      h.buf,
		minLevel: h.minLevel,
		attrs:    h.attrs,
		groups:   append(h.groups, name),
		done:     h.done,
	}
}

// Close flushes remaining logs and stops the background loop.
func (h *LogHandler) Close() {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.closed = true
	h.mu.Unlock()
	close(h.done)
	h.flush()
}

func (h *LogHandler) flushLoop() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-h.done:
			return
		case <-ticker.C:
			h.flush()
		}
	}
}

func (h *LogHandler) flush() {
	h.mu.Lock()
	if len(h.buf) == 0 {
		h.mu.Unlock()
		return
	}
	entries := h.buf
	h.buf = nil
	h.mu.Unlock()

	req := apitypes.LogBatchRequest{
		AgentName: h.cfg.AgentName,
		Entries:   entries,
	}
	body, err := json.Marshal(req)
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		h.cfg.ControlPlaneURL+"/api/v1/ingest/logs", bytes.NewReader(body))
	if err != nil {
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+h.cfg.APIKey)

	resp, err := h.client.Do(httpReq)
	if err != nil {
		return
	}
	resp.Body.Close()
}

// FormatLevel converts a slog level string to a short display form.
func FormatLevel(level string) string {
	switch level {
	case "DEBUG":
		return "DEBUG"
	case "INFO":
		return "INFO"
	case "WARN":
		return "WARN"
	case "ERROR":
		return "ERROR"
	default:
		return fmt.Sprintf("LVL(%s)", level)
	}
}
