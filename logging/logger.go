// Package logging builds the application's *slog.Logger from the [logging]
// config. It is cross-cutting: every layer may receive a *slog.Logger, but
// only the composition root calls New.
package logging

import (
	"io"
	"log/slog"
	"strings"

	"github.com/mihaiflorentin/wc3-team-follow/config"
)

// LoggerType names a log format accepted by [logging] default and
// server_default.
type LoggerType string

const (
	LoggerTypeSimple     LoggerType = "simple"
	LoggerTypeColor      LoggerType = "color"
	LoggerTypeJSON       LoggerType = "json"
	LoggerTypePrettyJSON LoggerType = "pretty-json"
)

// New returns a logger that writes to w using the format (cfg.Default) and
// minimum level (cfg.Level) from the logging config. A nil config, an unknown
// format or an unknown level fall back to the simple format at info level.
func New(cfg *config.LoggingConfig, w io.Writer) *slog.Logger {
	if cfg == nil {
		return newLogger(LoggerTypeSimple, "info", w)
	}
	return newLogger(LoggerType(cfg.Default), cfg.Level, w)
}

// NewAccessLogger returns the HTTP access logger: like New, but in the
// cfg.ServerDefault format, falling back to cfg.Default when it is empty.
func NewAccessLogger(cfg *config.LoggingConfig, w io.Writer) *slog.Logger {
	if cfg == nil || cfg.ServerDefault == "" {
		return New(cfg, w)
	}
	return newLogger(LoggerType(cfg.ServerDefault), cfg.Level, w)
}

func newLogger(format LoggerType, level string, w io.Writer) *slog.Logger {
	opts := &slog.HandlerOptions{Level: parseLevel(level)}
	switch format {
	case LoggerTypePrettyJSON:
		return slog.New(NewPrettyJSONHandler(w, opts))
	case LoggerTypeJSON:
		return slog.New(slog.NewJSONHandler(w, opts))
	case LoggerTypeColor:
		return slog.New(NewColorHandler(w, opts))
	default:
		return slog.New(NewCLIHandler(w, opts))
	}
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
