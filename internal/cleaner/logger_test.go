package cleaner

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestNewLoggerJSONFormat(t *testing.T) {
	var buf bytes.Buffer
	log := NewLogger(LoggerOptions{Format: LogFormatJSON, Output: &buf})
	log.Info("hello", "path", "main.go")

	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("output is not valid JSON: %v\nraw: %s", err, buf.String())
	}
	if rec["msg"] != "hello" {
		t.Errorf("msg = %v, want hello", rec["msg"])
	}
	if rec["path"] != "main.go" {
		t.Errorf("path = %v, want main.go", rec["path"])
	}
	if rec["level"] != "INFO" {
		t.Errorf("level = %v, want INFO", rec["level"])
	}
}

func TestNewLoggerConsoleFormat(t *testing.T) {
	var buf bytes.Buffer
	log := NewLogger(LoggerOptions{Format: LogFormatConsole, Output: &buf})
	log.Info("hello", "path", "main.go")

	out := buf.String()
	if !strings.Contains(out, "msg=hello") || !strings.Contains(out, "path=main.go") {
		t.Errorf("console output missing expected fields: %q", out)
	}
}

func TestNewLoggerLevels(t *testing.T) {
	tests := []struct {
		name        string
		opts        LoggerOptions
		wantDebug   bool
		wantInfo    bool
		wantWarning bool
	}{
		{"default", LoggerOptions{}, false, true, true},
		{"verbose", LoggerOptions{Verbose: true}, true, true, true},
		{"quiet", LoggerOptions{Quiet: true}, false, false, true},
		{"quiet wins over verbose", LoggerOptions{Quiet: true, Verbose: true}, false, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			tt.opts.Output = &buf
			tt.opts.Format = LogFormatConsole
			log := NewLogger(tt.opts)
			log.Debug("dbg")
			log.Info("inf")
			log.Warn("wrn")
			out := buf.String()

			if got := strings.Contains(out, "msg=dbg"); got != tt.wantDebug {
				t.Errorf("debug emitted = %v, want %v", got, tt.wantDebug)
			}
			if got := strings.Contains(out, "msg=inf"); got != tt.wantInfo {
				t.Errorf("info emitted = %v, want %v", got, tt.wantInfo)
			}
			if got := strings.Contains(out, "msg=wrn"); got != tt.wantWarning {
				t.Errorf("warn emitted = %v, want %v", got, tt.wantWarning)
			}
		})
	}
}

func TestNewLoggerDefaultsToStdout(t *testing.T) {
	// A nil Output must not panic; it falls back to os.Stdout.
	log := NewLogger(LoggerOptions{})
	if log == nil {
		t.Fatal("NewLogger returned nil")
	}
	log.Info("to stdout")
}
