// Copyright 2019 HAProxy Technologies LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package utils

import (
	"github.com/haproxytech/kubernetes-ingress/pkg/logger"
)

type LogLevel int8

const (
	Panic   LogLevel = 1
	Error   LogLevel = 2
	Warning LogLevel = 3
	Info    LogLevel = 4
	Debug   LogLevel = 5
	Trace   LogLevel = 6
)

const (
	LogFieldTransactionID = "transactionID"
)

// Logger wraps the modern Zap-based logger with backward-compatible methods
type Logger interface {
	logger.Logger
	// Backward-compatible formatted logging methods
	Tracef(format string, args ...interface{})
	Debugf(format string, args ...interface{})
	Infof(format string, args ...interface{})
	InfoSkipCallerf(format string, args ...interface{})
	Warning(args ...interface{})
	Warningf(format string, args ...interface{})
	Err(args ...interface{}) []error
	Errorf(format string, args ...interface{})
	Panic(args ...interface{})
	Panicf(format string, args ...interface{})
	WithField(key string, value interface{})
	ResetFields()
}

type loggerWrapper struct {
	logger.Logger
}

func (l *loggerWrapper) Tracef(format string, args ...interface{}) {
	l.Logger.Debugf(format, args...)
}

func (l *loggerWrapper) Debugf(format string, args ...interface{}) {
	l.Logger.Debugf(format, args...)
}

func (l *loggerWrapper) Infof(format string, args ...interface{}) {
	l.Logger.Infof(format, args...)
}

func (l *loggerWrapper) InfoSkipCallerf(format string, args ...interface{}) {
	l.Logger.Infof(format, args...)
}

func (l *loggerWrapper) Warning(args ...interface{}) {
	l.Logger.Warn(args...)
}

func (l *loggerWrapper) Warningf(format string, args ...interface{}) {
	l.Logger.Warnf(format, args...)
}

func (l *loggerWrapper) Err(args ...interface{}) []error {
	// Log the errors
	l.Logger.Error(args...)

	// Extract and return actual error objects
	result := []error{}
	for _, d := range args {
		if d == nil {
			continue
		}
		err, ok := d.(error)
		if ok {
			result = append(result, err)
		}
	}
	if len(result) > 0 {
		return result
	}
	return nil
}

func (l *loggerWrapper) Errorf(format string, args ...interface{}) {
	l.Logger.Errorf(format, args...)
}

func (l *loggerWrapper) Panic(args ...interface{}) {
	l.Logger.Fatal(args...)
}

func (l *loggerWrapper) Panicf(format string, args ...interface{}) {
	l.Logger.Fatalf(format, args...)
}

func (l *loggerWrapper) WithField(key string, value interface{}) {
	l.Logger = l.Logger.With(key, value).(logger.Logger)
}

func (l *loggerWrapper) ResetFields() {
	// For the wrapper, we can't truly reset fields on the underlying logger
	// This is a no-op for backward compatibility
	// In practice, callers should create a new logger instance instead
}

// GetLogger returns a logger instance
// Use logger.New() instead for module-specific loggers
func GetLogger() Logger {
	return &loggerWrapper{Logger: logger.Log}
}

// GetK8sLogger returns a logger for Kubernetes-related operations
func GetK8sLogger() Logger {
	return &loggerWrapper{Logger: logger.New("k8s", "info")}
}
