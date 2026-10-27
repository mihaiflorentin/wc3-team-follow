package gui

import (
	"log/slog"
	"os"
)

// wailsLog passes Wails' own messages to the app's logger as gui.wails,
// so the log file shows how far the window got (web view start-up, process
// failures) instead of nothing.
type wailsLog struct{ logger *slog.Logger }

func (w wailsLog) Print(message string) { w.logger.Info("gui.wails", slog.String("message", message)) }
func (w wailsLog) Trace(message string) { w.logger.Debug("gui.wails", slog.String("message", message)) }
func (w wailsLog) Debug(message string) { w.logger.Debug("gui.wails", slog.String("message", message)) }
func (w wailsLog) Info(message string)  { w.logger.Info("gui.wails", slog.String("message", message)) }
func (w wailsLog) Warning(message string) {
	w.logger.Warn("gui.wails", slog.String("message", message))
}
func (w wailsLog) Error(message string) { w.logger.Error("gui.wails", slog.String("message", message)) }

// Fatal logs and exits, as Wails expects; the crash reporter shows the
// message (the "error:" line) to the player.
func (w wailsLog) Fatal(message string) {
	w.logger.Error("gui.wails", slog.String("message", message))
	_, _ = os.Stderr.WriteString("error: " + message + "\n")
	os.Exit(1)
}
