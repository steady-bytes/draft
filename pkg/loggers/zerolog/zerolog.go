package zerolog

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/steady-bytes/draft/pkg/chassis"

	"github.com/rs/zerolog"
)

type (
	Logger interface {
		chassis.Logger
		// Logger returns the underlying zerolog.Logger
		Logger() zerolog.Logger
	}

	logger struct {
		logger zerolog.Logger
		fields chassis.Fields
		level  chassis.LogLevel
	}
)

func New() chassis.Logger {
	return &logger{
		fields: make(chassis.Fields),
	}
}

// TODO: need to rethink this. maybe add the hook to the interface: GetHook()?
// CreateNullLogger creates a logger for testing that wraps the null logger provided by logrus
// func CreateNullLogger() (chassis.Logger, *test.Hook) {
// 	nullLogger, logHook := test.NewNullLogger()
// 	return newLogger(nullLogger.WithField("", "")), logHook
// }

func (l *logger) Logger() zerolog.Logger {
	return l.logger
}

func (l *logger) Start(config chassis.Config) {
	zl := zerolog.New(os.Stdout).With().Str("service", config.Name()).Timestamp().Logger()
	if config.Env() == "local" || config.Env() == "test" {
		zl = zl.Output(zerolog.ConsoleWriter{Out: os.Stdout})
	}
	l.level = chassis.ParseLogLevel(config.GetString("service.logging.level"))
	l.logger = zl.Level(parseLevel(l.level))
}

func (l *logger) SetLevel(level chassis.LogLevel) {
	l.logger = l.logger.Level(parseLevel(level))
	l.level = level
}

func (l *logger) GetLevel() chassis.LogLevel {
	return l.level
}

func (l *logger) Wrap(err error) error {
	return chassis.Wrap(err, l.fields)
}

func (l *logger) WithError(err error) chassis.Logger {
	return &logger{
		logger: (l.logger.With().Str("error", err.Error()).Logger()),
		fields: l.fields,
		level:  l.level,
	}
}

func (l *logger) WithContext(ctx context.Context) chassis.Logger {
	// TODO: is this really just for tracing?
	return l
}

func (l *logger) WithField(key string, value any) chassis.Logger {
	return l.WithFields(chassis.Fields{key: value})
}

func (l *logger) WithFields(fields chassis.Fields) chassis.Logger {
	newFields := make(chassis.Fields, len(l.fields)+len(fields))
	// copy old fields
	for k, v := range l.fields {
		newFields[k] = v
	}
	// copy old logger
	new := &logger{
		logger: l.logger.With().Logger(),
		fields: newFields,
	}
	// append new fields
	for key, value := range fields {
		str := fmt.Sprintf("%v", value)
		new.fields[key] = str
		new.logger = new.logger.With().Str(key, str).Logger()
	}

	return new
}

// Implement `log.Logger` for `envoyproxy/go-control-plane/pkg/cache/v3`
// go-control-plane has it's own logger interface that needs to be implemented for logging
// to work correctly
func (l *logger) Debugf(format string, args ...any) {
	l.correctFunctionName().logger.Debug().Msgf(format, args...)
}

func (l *logger) Infof(format string, args ...any) {
	l.correctFunctionName().logger.Info().Msgf(format, args...)
}

func (l *logger) Warnf(format string, args ...any) {
	l.correctFunctionName().logger.Warn().Msgf(format, args...)
}

func (l *logger) Errorf(format string, args ...any) {
	l.correctFunctionName().logger.Error().Msgf(format, args...)
}

// Default `draft.Logger` interface implementations
func (l *logger) Trace(msg string) {
	l.correctFunctionName().logger.Trace().Msg(msg)
}

func (l *logger) Debug(msg string) {
	l.correctFunctionName().logger.Debug().Msg(msg)
}

func (l *logger) Info(msg string) {
	l.correctFunctionName().logger.Info().Msg(msg)
}

func (l *logger) Warn(msg string) {
	l.correctFunctionName().logger.Warn().Msg(msg)
}

func (l *logger) Error(msg string) {
	l.correctFunctionName().logger.Error().Msg(msg)
}

func (l *logger) WrappedError(err error, msg string) {
	e := chassis.Wrap(err, chassis.Fields(l.fields))
	n := &logger{
		logger: l.logger,
		fields: e.Fields(),
	}
	for key, value := range e.Fields() {
		n.logger = n.logger.With().Str(key, fmt.Sprintf("%v", value)).Logger()
	}
	n.logger = n.logger.With().Str("error", e.Error()).Logger()
	n.correctFunctionName().logger.Error().Msg(msg)
}

func (l *logger) Fatal(msg string) {
	l.correctFunctionName().logger.Fatal().Msg(msg)
}

func (l *logger) Panic(msg string) {
	l.correctFunctionName().logger.Panic().Msg(msg)
}

func (l *logger) correctFunctionName() *logger {
	pc, _, _, ok := runtime.Caller(2)
	if !ok {
		return nil
	}
	fn := runtime.FuncForPC(pc).Name()
	functionName := fn[strings.LastIndex(fn, ".")+1:]
	return &logger{
		logger: l.logger.With().Str("function", functionName).Logger(),
	}
}

// parseLevel converts a chassis.Level to a zerolog.Level and defaults
// to zerolog.PanicLevel if the conversion is defined.
func parseLevel(lvl chassis.LogLevel) zerolog.Level {
	switch lvl {
	case chassis.PanicLevel:
		return zerolog.PanicLevel
	case chassis.FatalLevel:
		return zerolog.FatalLevel
	case chassis.ErrorLevel:
		return zerolog.ErrorLevel
	case chassis.WarnLevel:
		return zerolog.WarnLevel
	case chassis.InfoLevel:
		return zerolog.InfoLevel
	case chassis.DebugLevel:
		return zerolog.DebugLevel
	case chassis.TraceLevel:
		return zerolog.TraceLevel
	default:
		return zerolog.PanicLevel
	}
}
