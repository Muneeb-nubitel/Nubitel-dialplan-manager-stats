package logging

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	defaultMaxSizeMB  = 20
	defaultMaxAgeDays = 10
)

type Handler struct {
	writer   io.Writer
	level    slog.Leveler
	hostname string
	attrs    []slog.Attr
	group    string
	mu       *sync.Mutex
}

func NewHandler(writer io.Writer, level slog.Leveler) *Handler {
	hostname, _ := os.Hostname()
	return &Handler{writer: writer, level: level, hostname: hostname, mu: &sync.Mutex{}}
}

func (h *Handler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level.Level()
}

func (h *Handler) Handle(_ context.Context, record slog.Record) error {
	fields := make([]string, 0, record.NumAttrs()+len(h.attrs))
	metadata := map[string]any{"pid": os.Getpid(), "hostname": h.hostname}

	appendAttr := func(attr slog.Attr) {
		attr.Value = attr.Value.Resolve()
		key := attr.Key
		if h.group != "" {
			key = h.group + "." + key
		}
		if key == "request_id" || key == "rid" {
			metadata["rid"] = attr.Value.Any()
			return
		}
		fields = append(fields, fmt.Sprintf("%s=%v", key, attr.Value.Any()))
	}

	for _, attr := range h.attrs {
		appendAttr(attr)
	}
	record.Attrs(func(attr slog.Attr) bool {
		appendAttr(attr)
		return true
	})

	meta, err := json.Marshal(metadata)
	if err != nil {
		return err
	}

	message := record.Message
	if len(fields) > 0 {
		message += " " + strings.Join(fields, " || ")
	}
	line := fmt.Sprintf("%s\t%s\t%s\t%s\n",
		record.Time.Format("2006-01-02T15:04:05.000-0700"),
		strings.ToLower(record.Level.String()),
		message,
		meta,
	)

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err = io.WriteString(h.writer, line)
	return err
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := *h
	clone.attrs = append(append([]slog.Attr(nil), h.attrs...), attrs...)
	return &clone
}

func (h *Handler) WithGroup(name string) slog.Handler {
	clone := *h
	if clone.group == "" {
		clone.group = name
	} else {
		clone.group += "." + name
	}
	return &clone
}

var _ slog.Handler = (*Handler)(nil)

func New(level slog.Leveler) *slog.Logger {
	logFile := fmt.Sprintf(
		"logs/dialplan-manager-stats-%s-pid%d.log",
		time.Now().Format("20060102-150405"),
		os.Getpid(),
	)
	rotatingFile := &lumberjack.Logger{
		Filename:  logFile,
		MaxSize:   defaultMaxSizeMB,
		MaxAge:    defaultMaxAgeDays,
		LocalTime: true,
		Compress:  true,
	}
	return slog.New(NewHandler(io.MultiWriter(os.Stdout, rotatingFile), level))
}
