package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type LoggerSeverity string

const (
	INFO  LoggerSeverity = "INFO"  // This is a basic informational alert.
	WARN  LoggerSeverity = "WARN"  // This is a warning, meaning the program has recovered from an error.
	DEBUG LoggerSeverity = "DEBUG" // This is a detailed alert containing information used for debugging.
	ERROR LoggerSeverity = "ERROR" // An error has occurred, please advise.
	FATAL LoggerSeverity = "FATAL" // An irrecoverable error has occured and the program must exit immediately.
)

var (
	LoggerInit     = &LoggerInstance{source: "INIT"}
	LoggerHTTP     = &LoggerInstance{source: "HTTP"}
	LoggerModel    = &LoggerInstance{source: "ONNX"}
	LoggerStorage  = &LoggerInstance{source: "DATA"}
	LoggerDatabase = &LoggerInstance{source: "PGDB"}
)

type LoggerInstance struct {
	source string
}

func (p *LoggerInstance) entry(severity LoggerSeverity, source, message string) {
	target := os.Stdout
	if severity == ERROR || severity == FATAL {
		target = os.Stderr
	}
	fmt.Fprintf(target, "%s [%s] [%s] %s\n", time.Now().Format(time.DateTime), severity, source, message)
}

func (p *LoggerInstance) Log(severity LoggerSeverity, format string, a ...any) {
	p.entry(severity, p.source, fmt.Sprintf(format, a...))
	if severity == FATAL {
		os.Exit(1)
	}
}

func (p *LoggerInstance) Data(severity LoggerSeverity, message string, data any) {
	if data == nil {
		p.entry(severity, p.source, message)
	} else {
		entryData := ""
		if b, err := json.MarshalIndent(data, "", "  "); err != nil {
			entryData = fmt.Sprintf("marshal_error: %q", err)
		} else {
			entryData = string(b)
		}
		p.entry(severity, p.source, fmt.Sprintf("%s\n%s\n---", message, entryData))
	}
	if severity == FATAL {
		os.Exit(1)
	}
}
