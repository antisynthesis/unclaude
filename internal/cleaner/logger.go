package cleaner

import (
	"io"
	"log/slog"
	"os"
)

// LogFormat selects the encoding used by the logger returned from NewLogger.
type LogFormat int

const (
	// LogFormatConsole emits human-readable key=value lines (slog's text handler).
	LogFormatConsole LogFormat = iota
	// LogFormatJSON emits one JSON object per record (slog's JSON handler).
	LogFormatJSON
)

// LoggerOptions configure NewLogger.
type LoggerOptions struct {
	// Verbose lowers the level to debug. Ignored when Quiet is set.
	Verbose bool
	// Quiet raises the level to warn, suppressing info and debug records.
	Quiet bool
	// Format selects the console (text) or JSON encoder.
	Format LogFormat
	// Output is where records are written. Defaults to os.Stdout when nil.
	Output io.Writer
}

// NewLogger returns a slog.Logger writing to opts.Output (or os.Stdout).
// The level follows Verbose/Quiet: verbose -> debug, quiet -> warn, otherwise
// info. Both built-in handlers timestamp records in RFC3339 form.
func NewLogger(opts LoggerOptions) *slog.Logger {
	out := opts.Output
	if out == nil {
		out = os.Stdout
	}

	level := slog.LevelInfo
	switch {
	case opts.Quiet:
		level = slog.LevelWarn
	case opts.Verbose:
		level = slog.LevelDebug
	}

	handlerOpts := &slog.HandlerOptions{Level: level}
	var handler slog.Handler
	switch opts.Format {
	case LogFormatJSON:
		handler = slog.NewJSONHandler(out, handlerOpts)
	default:
		handler = slog.NewTextHandler(out, handlerOpts)
	}
	return slog.New(handler)
}

// newNopLogger returns a logger that discards every record. It backs the
// zero-value logger path in New so callers never have to nil-check.
func newNopLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}
