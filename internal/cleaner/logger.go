package cleaner

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// LogFormat selects the encoder for the logger.
type LogFormat int

const (
	LogFormatConsole LogFormat = iota
	LogFormatJSON
)

// LoggerOptions configure NewLogger.
type LoggerOptions struct {
	Verbose bool
	Quiet   bool
	Format  LogFormat
}

// NewLogger returns a zap logger with RFC3339 timestamps writing to stdout.
// Level follows Verbose/Quiet: verbose → debug, quiet → warn, default → info.
func NewLogger(opts LoggerOptions) *zap.Logger {
	encCfg := zap.NewProductionEncoderConfig()
	encCfg.TimeKey = "time"
	encCfg.EncodeTime = zapcore.RFC3339TimeEncoder
	encCfg.EncodeDuration = zapcore.StringDurationEncoder

	var encoder zapcore.Encoder
	switch opts.Format {
	case LogFormatJSON:
		encoder = zapcore.NewJSONEncoder(encCfg)
	default:
		encCfg.EncodeLevel = zapcore.CapitalLevelEncoder
		encoder = zapcore.NewConsoleEncoder(encCfg)
	}

	level := zap.InfoLevel
	switch {
	case opts.Quiet:
		level = zap.WarnLevel
	case opts.Verbose:
		level = zap.DebugLevel
	}

	core := zapcore.NewCore(encoder, zapcore.AddSync(os.Stdout), level)
	return zap.New(core)
}
