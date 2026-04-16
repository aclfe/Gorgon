package logger

import (
	"fmt"
	"io"
	"os"
)

// Logger is a lightweight, levelled writer for Gorgon's diagnostic output.
// All output goes to Stderr by default. When a debug-file writer is set via
// SetDebugFile, Info, Debug, and Warn messages are mirrored there too.
type Logger struct {
	debug bool
	out   io.Writer // normally os.Stderr
	file  io.Writer // optional debug-file mirror; nil by default
}

// New returns a Logger that enables Debug messages only when debug is true.
func New(debug bool) *Logger {
	return &Logger{debug: debug, out: os.Stderr}
}

// SetDebugFile mirrors all future log output to w (e.g. an open *os.File).
func (l *Logger) SetDebugFile(w io.Writer) { l.file = w }

// IsDebug reports whether debug logging is enabled.
func (l *Logger) IsDebug() bool { return l.debug }

// Info always prints — use for progress and notable operational events.
func (l *Logger) Info(format string, args ...any) {
	l.emit(l.out, "[INFO] ", format, args...)
	if l.file != nil {
		l.emit(l.file, "[INFO] ", format, args...)
	}
}

// Debug prints only when debug mode is active.
func (l *Logger) Debug(format string, args ...any) {
	if !l.debug {
		return
	}
	l.emit(l.out, "[DEBUG] ", format, args...)
	if l.file != nil {
		l.emit(l.file, "[DEBUG] ", format, args...)
	}
}

// Warn always prints — use for non-fatal anomalies.
func (l *Logger) Warn(format string, args ...any) {
	l.emit(l.out, "[WARN] ", format, args...)
	if l.file != nil {
		l.emit(l.file, "[WARN] ", format, args...)
	}
}

// Print writes a raw line with no level prefix — for plain progress output.
func (l *Logger) Print(format string, args ...any) {
	if len(args) > 0 {
		fmt.Fprintf(l.out, format+"\n", args...)
	} else {
		fmt.Fprintln(l.out, format)
	}
}

func (l *Logger) emit(w io.Writer, prefix, format string, args ...any) {
	if len(args) > 0 {
		fmt.Fprintf(w, prefix+format+"\n", args...)
	} else {
		fmt.Fprintln(w, prefix+format)
	}
}
