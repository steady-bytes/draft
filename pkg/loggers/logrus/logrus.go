package logrus

import (
	"context"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/steady-bytes/draft/pkg/chassis"

	"github.com/sirupsen/logrus"
)

type (
	Logger interface {
		chassis.Logger
		// Entry returns the underlying logrus.Entry
		Entry() *logrus.Entry
	}

	logger struct {
		entry *logrus.Entry
	}
)

func New() chassis.Logger {
	return &logger{}
}

// TODO: need to rethink this. maybe add the hook to the interface: GetHook()?
// CreateNullLogger creates a logger for testing that wraps the null logger provided by logrus
// func CreateNullLogger() (chassis.Logger, *test.Hook) {
// 	nullLogger, logHook := test.NewNullLogger()
// 	return newLogger(nullLogger.WithField("", "")), logHook
// }

func (l *logger) Entry() *logrus.Entry {
	return l.entry
}

func (l *logger) Start(config chassis.Config) {
	if config.Env() != "local" {
		logrus.SetFormatter(&Formatter{
			Line:    true,
			Package: true,
			File:    true,
			ChildFormatter: &logrus.JSONFormatter{
				DisableHTMLEscape: true,
			},
		})
	}
	logrus.SetOutput(os.Stdout)
	l.entry = logrus.WithField("service", config.Name())
	levelString := config.GetString("service.logging.level")
	l.entry.Logger.SetLevel(logrus.Level(chassis.ParseLogLevel(levelString)))
}

func newLogger(e *logrus.Entry) chassis.Logger {
	return &logger{entry: e}
}

func (l *logger) SetLevel(level chassis.LogLevel) {
	l.entry.Logger.SetLevel(logrus.Level(level))
}

func (l *logger) GetLevel() chassis.LogLevel {
	return chassis.LogLevel(l.entry.Level)
}

func (l *logger) Wrap(err error) error {
	return chassis.Wrap(err, chassis.Fields(l.entry.Data))
}

func (l *logger) WithError(err error) chassis.Logger {
	return newLogger(l.entry.WithError(err))
}

func (l *logger) WithContext(ctx context.Context) chassis.Logger {
	return newLogger(l.entry.WithContext(ctx))
}

func (l *logger) WithField(key string, value interface{}) chassis.Logger {
	return newLogger(l.entry.WithField(key, value))
}

func (l *logger) WithFields(fields chassis.Fields) chassis.Logger {
	logrusFields := logrus.Fields{}
	for index, element := range fields {
		logrusFields[index] = element
	}
	return newLogger(l.entry.WithFields(logrusFields))
}

// Implement `log.Logger` for `envoyproxy/go-control-plane/pkg/cache/v3`
// go-control-plane has it's own logger interface that needs to be implemented for logging
// to work correctly

// dropping the args parameter for now, as it is not used
func (l *logger) Debugf(msg string, args ...any) {
	l.correctFunctionName().entry.Debug(msg)
}

// dropping the args parameter for now, as it is not used
func (l *logger) Infof(msg string, args ...any) {
	l.correctFunctionName().entry.Info(msg)
}

// dropping the args parameter for now, as it is not used
func (l *logger) Warnf(msg string, args ...any) {
	l.correctFunctionName().entry.Warn(msg)
}

// dropping the args parameter for now, as it is not used
func (l *logger) Errorf(msg string, args ...any) {
	l.correctFunctionName().entry.Error(msg)
}

func (l *logger) WithTime(t time.Time) chassis.Logger {
	return newLogger(l.entry.WithTime(t))
}

func (l *logger) Trace(msg string) {
	l.correctFunctionName().entry.Trace(msg)
}

func (l *logger) Debug(msg string) {
	l.correctFunctionName().entry.Debug(msg)
}

func (l *logger) Info(msg string) {
	l.correctFunctionName().entry.Info(msg)
}

func (l *logger) Warn(msg string) {
	l.correctFunctionName().entry.Warn(msg)
}

func (l *logger) Error(msg string) {
	l.correctFunctionName().entry.Error(msg)
}

func (l *logger) WrappedError(err error, msg string) {
	e := chassis.Wrap(err, chassis.Fields(l.entry.Data))
	l.correctFunctionName().entry.WithFields(logrus.Fields(e.Fields())).WithError(e).Error(msg)
}

func (l *logger) Fatal(msg string) {
	l.correctFunctionName().entry.Fatal(msg)
}

func (l *logger) Panic(msg string) {
	l.correctFunctionName().entry.Panic(msg)
}

func (l *logger) correctFunctionName() *logger {
	pc, _, _, ok := runtime.Caller(2)
	if !ok {
		return nil
	}
	fn := runtime.FuncForPC(pc).Name()
	functionName := fn[strings.LastIndex(fn, ".")+1:]
	return &logger{
		entry: l.entry.WithField("function", functionName),
	}
}
