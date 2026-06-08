package logger

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// Level represents the severity of a log entry.
type Level int

const (
	LevelDebug Level = iota - 1
	LevelInfo
	LevelWarn
	LevelError
)

var levelNames = map[Level]string{
	LevelDebug: "DEBUG",
	LevelInfo:  "INFO",
	LevelWarn:  "WARN",
	LevelError: "ERROR",
}

func levelFromString(s string) Level {
	switch s {
	case "debug":
		return LevelDebug
	case "info":
		return LevelInfo
	case "warn":
		return LevelWarn
	case "error":
		return LevelError
	default:
		return LevelInfo
	}
}

// Logger is a JSON structured logger.
type Logger struct {
	mu     sync.Mutex
	level  Level
	output io.Writer
	fields map[string]interface{}
}

// entry is the JSON structure written for each log line.
type entry struct {
	Level   string                 `json:"level"`
	Time    string                 `json:"time"`
	Message string                 `json:"message"`
	Fields  map[string]interface{} `json:"fields,omitempty"`
}

// New creates a new Logger with the given level name and output writer.
// Valid level names: "debug", "info", "warn", "error".
// If output is nil, os.Stdout is used.
func New(levelName string, output io.Writer) *Logger {
	if output == nil {
		output = os.Stdout
	}
	return &Logger{
		level:  levelFromString(levelName),
		output: output,
		fields: make(map[string]interface{}),
	}
}

// With returns a new Logger that includes the given key-value fields on
// every entry. The original Logger is unchanged.
func (l *Logger) With(keyValues ...interface{}) *Logger {
	if len(keyValues)%2 != 0 {
		keyValues = append(keyValues, "(MISSING)")
	}
	cp := &Logger{
		level:  l.level,
		output: l.output,
		fields: make(map[string]interface{}, len(l.fields)+len(keyValues)/2),
	}
	for k, v := range l.fields {
		cp.fields[k] = v
	}
	for i := 0; i < len(keyValues); i += 2 {
		key, ok := keyValues[i].(string)
		if !ok {
			key = fmt.Sprintf("%v", keyValues[i])
		}
		cp.fields[key] = keyValues[i+1]
	}
	return cp
}

func (l *Logger) log(level Level, msg string, keyValues ...interface{}) {
	if level < l.level {
		return
	}

	fields := make(map[string]interface{})
	for k, v := range l.fields {
		fields[k] = v
	}
	for i := 0; i < len(keyValues); i += 2 {
		key, ok := keyValues[i].(string)
		if !ok {
			key = fmt.Sprintf("%v", keyValues[i])
		}
		if i+1 < len(keyValues) {
			fields[key] = keyValues[i+1]
		} else {
			fields[key] = "(MISSING)"
		}
	}

	e := entry{
		Level:   levelNames[level],
		Time:    time.Now().UTC().Format(time.RFC3339Nano),
		Message: msg,
	}

	if len(fields) > 0 {
		e.Fields = fields
	}

	data, err := json.Marshal(e)
	if err != nil {
		return // unreachable in practice
	}
	data = append(data, '\n')

	l.mu.Lock()
	_, _ = l.output.Write(data)
	l.mu.Unlock()
}

// Debug logs a message at debug level.
func (l *Logger) Debug(msg string, keyValues ...interface{}) {
	l.log(LevelDebug, msg, keyValues...)
}

// Info logs a message at info level.
func (l *Logger) Info(msg string, keyValues ...interface{}) {
	l.log(LevelInfo, msg, keyValues...)
}

// Warn logs a message at warn level.
func (l *Logger) Warn(msg string, keyValues ...interface{}) {
	l.log(LevelWarn, msg, keyValues...)
}

// Error logs a message at error level.
func (l *Logger) Error(msg string, keyValues ...interface{}) {
	l.log(LevelError, msg, keyValues...)
}

// Level returns the current log level.
func (l *Logger) Level() Level {
	return l.level
}
