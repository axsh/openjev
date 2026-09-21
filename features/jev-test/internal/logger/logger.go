package logger

import (
	"context"
	"io"
	"log/slog"
)

const levelTrace = slog.Level(-8)

type Logger struct {
	s *slog.Logger
}

func New(w io.Writer) *Logger {
	handler := slog.NewTextHandler(w, &slog.HandlerOptions{Level: levelTrace})
	return &Logger{s: slog.New(handler)}
}

func (l *Logger) WithComponent(name string) *Logger {
	return &Logger{s: l.s.With("component", name)}
}

func (l *Logger) Info(msg string, kv ...any)  { l.s.Info(msg, kv...) }
func (l *Logger) Warn(msg string, kv ...any)  { l.s.Warn(msg, kv...) }
func (l *Logger) Error(msg string, kv ...any) { l.s.Error(msg, kv...) }
func (l *Logger) Debug(msg string, kv ...any) { l.s.Debug(msg, kv...) }

func (l *Logger) Trace(msg string, kv ...any) {
	l.s.Log(context.Background(), levelTrace, msg, kv...)
}
