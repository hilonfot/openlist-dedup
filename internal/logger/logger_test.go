package logger

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	var buf bytes.Buffer
	log := New("info", &buf)
	if log.level != LevelInfo {
		t.Errorf("expected LevelInfo, got %v", log.level)
	}
	if log.output == nil {
		t.Error("expected non-nil output")
	}

	// nill output defaults to os.Stdout
	log = New("debug", nil)
	if log.output == nil {
		t.Error("expected non-nil output for nil input")
	}
}

func TestDebug_LevelFiltered(t *testing.T) {
	var buf bytes.Buffer
	log := New("info", &buf)
	log.Debug("should not appear")
	if buf.Len() > 0 {
		t.Error("expected no output for debug message at info level")
	}
}

func TestInfo_LevelPassed(t *testing.T) {
	var buf bytes.Buffer
	log := New("info", &buf)
	log.Info("hello world")

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if entry["level"] != "INFO" {
		t.Errorf("expected level INFO, got %v", entry["level"])
	}
	if entry["message"] != "hello world" {
		t.Errorf("expected message hello world, got %v", entry["message"])
	}
	if _, ok := entry["time"]; !ok {
		t.Error("expected time field")
	}
}

func TestWarn_WithFields(t *testing.T) {
	var buf bytes.Buffer
	log := New("debug", &buf)
	log.Warn("something went wrong", "err", "timeout", "retry", 3)

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if entry["level"] != "WARN" {
		t.Errorf("expected level WARN, got %v", entry["level"])
	}
	if entry["message"] != "something went wrong" {
		t.Errorf("expected message, got %v", entry["message"])
	}

	fields, ok := entry["fields"].(map[string]interface{})
	if !ok {
		t.Fatal("expected fields object")
	}
	if fields["err"] != "timeout" {
		t.Errorf("expected err=timeout, got %v", fields["err"])
	}
	if fields["retry"] != float64(3) {
		t.Errorf("expected retry=3, got %v", fields["retry"])
	}
}

func TestError(t *testing.T) {
	var buf bytes.Buffer
	log := New("debug", &buf)
	log.Error("fatal error", "code", 500)

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if entry["level"] != "ERROR" {
		t.Errorf("expected level ERROR, got %v", entry["level"])
	}
}

func TestWith(t *testing.T) {
	var buf bytes.Buffer
	log := New("debug", &buf)
	log = log.With("app", "openlist", "version", "1.0")
	log.Info("started")

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	fields, ok := entry["fields"].(map[string]interface{})
	if !ok {
		t.Fatal("expected fields object")
	}
	if fields["app"] != "openlist" {
		t.Errorf("expected app=openlist, got %v", fields["app"])
	}
	if fields["version"] != "1.0" {
		t.Errorf("expected version=1.0, got %v", fields["version"])
	}
}

func TestWith_PreservesImmutable(t *testing.T) {
	var buf bytes.Buffer
	base := New("debug", &buf)
	child := base.With("extra", "value")
	base.Info("base message")
	child.Info("child message")

	// Both entries go to the same buffer; parse them line by line
	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 log lines, got %d", len(lines))
	}

	var baseEntry, childEntry map[string]interface{}
	if err := json.Unmarshal([]byte(lines[0]), &baseEntry); err != nil {
		t.Fatalf("failed to parse base json: %v", err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &childEntry); err != nil {
		t.Fatalf("failed to parse child json: %v", err)
	}

	// base should not have "extra"
	if _, ok := baseEntry["fields"]; ok {
		fields := baseEntry["fields"].(map[string]interface{})
		if _, ok := fields["extra"]; ok {
			t.Error("base logger should not have extra field")
		}
	}

	// child should have "extra"
	fields := childEntry["fields"].(map[string]interface{})
	if fields["extra"] != "value" {
		t.Errorf("expected child to have extra=value, got %v", fields["extra"])
	}
}

func TestJSONFormat(t *testing.T) {
	var buf bytes.Buffer
	log := New("info", &buf)
	log.Info("test")

	// Verify the output is valid JSON with the expected fields
	var raw map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	required := []string{"level", "time", "message"}
	for _, field := range required {
		if _, ok := raw[field]; !ok {
			t.Errorf("missing required field: %s", field)
		}
	}
}

func TestLogger_NewLine(t *testing.T) {
	var buf bytes.Buffer
	log := New("info", &buf)
	log.Info("test")

	// Each log line should end with a newline
	if !strings.HasSuffix(buf.String(), "\n") {
		t.Error("log entry should end with newline")
	}
}

func TestLevelFromString(t *testing.T) {
	tests := []struct {
		input string
		want  Level
	}{
		{"debug", LevelDebug},
		{"info", LevelInfo},
		{"warn", LevelWarn},
		{"error", LevelError},
		{"unknown", LevelInfo},
	}
	for _, tt := range tests {
		got := levelFromString(tt.input)
		if got != tt.want {
			t.Errorf("levelFromString(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestOddKeyValues(t *testing.T) {
	var buf bytes.Buffer
	log := New("debug", &buf)
	log.Info("odd", "key")

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	fields, ok := entry["fields"].(map[string]interface{})
	if !ok {
		t.Fatal("expected fields")
	}
	if fields["key"] != "(MISSING)" {
		t.Errorf("expected (MISSING) for odd key, got %v", fields["key"])
	}
}

func TestLevelConstants(t *testing.T) {
	if LevelDebug >= LevelInfo {
		t.Error("LevelDebug should be less than LevelInfo")
	}
	if LevelInfo >= LevelWarn {
		t.Error("LevelInfo should be less than LevelWarn")
	}
	if LevelWarn >= LevelError {
		t.Error("LevelWarn should be less than LevelError")
	}
}
