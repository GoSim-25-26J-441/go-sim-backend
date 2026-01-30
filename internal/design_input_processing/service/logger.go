package service

import (
	"context"
	"log"
)

// Logger provides structured logging for services
type Logger struct {
	requestID string
}

// NewLogger creates a logger with request context
func NewLogger(ctx context.Context) *Logger {
	// Try to get request ID from context (set by middleware)
	requestID := "unknown"
	if rid, ok := ctx.Value("request_id").(string); ok && rid != "" {
		requestID = rid
	}
	return &Logger{requestID: requestID}
}

// LogError logs an error with context
func (l *Logger) LogError(operation string, err error) {
	log.Printf("[error] request_id=%s operation=%s error=%v", l.requestID, operation, err)
}

// LogErrorf logs a formatted error with context
func (l *Logger) LogErrorf(operation string, format string, args ...interface{}) {
	log.Printf("[error] request_id=%s operation=%s "+format, append([]interface{}{l.requestID, operation}, args...)...)
}

// LogInfo logs an info message with context
func (l *Logger) LogInfo(operation string, message string) {
	log.Printf("[info] request_id=%s operation=%s message=%s", l.requestID, operation, message)
}

// LogInfof logs a formatted info message with context
func (l *Logger) LogInfof(operation string, format string, args ...interface{}) {
	log.Printf("[info] request_id=%s operation=%s "+format, append([]interface{}{l.requestID, operation}, args...)...)
}

// LogWarn logs a warning with context
func (l *Logger) LogWarn(operation string, message string) {
	log.Printf("[warn] request_id=%s operation=%s message=%s", l.requestID, operation, message)
}

// LogWarnf logs a formatted warning with context
func (l *Logger) LogWarnf(operation string, format string, args ...interface{}) {
	log.Printf("[warn] request_id=%s operation=%s "+format, append([]interface{}{l.requestID, operation}, args...)...)
}
