package logging

import (
	"context"
	"runtime"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// Logger is an interface wrapping around logrus to provide
// context based logging for tracing and wrapped error handling
// while simplifying the logrus interface
type Logger interface {
	// AddGlobalField adds a key:value pair to every log written with this logger
	// This is helpful for service-wide values
	AddGlobalField(key string, val string)
	// SetLevel sets the logging level for the logger
	SetLevel(level logrus.Level)
	// GetEntry returns a copy of the logrus Entry used in the logger
	// NOTE: Only use the base logrus.Entry when absolutely necessary.
	// Logging should really be done through the CustomLogger wrapper,
	// NOT through logrus directly.
	GetEntry() *logrus.Entry
	// WrapError wraps a standard Go error (OR `logging.Error`) into a custom error with the
	// additional context of current logger fields and call stack information.
	WrapError(error) Error
	// WithError - Add an error as single field (using the key defined in ErrorKey) to the Entry.
	WithError(err error) *logger
	// WithContext - Add a context to the Entry.
	WithContext(ctx context.Context) *logger
	// WithField - Add a single field to the Entry.
	WithField(key string, value interface{}) *logger
	// WithFields - Add a map of fields to the Entry.
	WithFields(fields Fields) *logger
	// WithTime - Overrides the time of the Entry.
	WithTime(t time.Time) *logger
	// Trace - Definition:
	// "Seriously, WTF is going on here?!?!
	// I need to log every single statement I execute to find this @#$@ing memory corruption bug before I go insane"
	Trace(msg string)
	// Debug - Definition:
	// Off by default, able to be turned on for debugging specific unexpected problems.
	// This is where you might log detailed information about key method parameters or
	// other information that is useful for finding likely problems in specific 'problematic' areas of the code.
	Debug(msg string)
	// Info - Definition:
	// Normal logging that's part of the normal operation of the app;
	// diagnostic stuff so you can go back and say 'how often did this broad-level operation happen?',
	// or 'how did the user's data get into this state?'
	Info(msg string)
	// Warn - Definition:
	// something that's concerning but not causing the operation to abort;
	// # of connections in the DB pool getting low, an unusual-but-expected timeout in an operation, etc.
	// Think of 'WARN' as something that's useful in aggregate; e.g. grep, group,
	// and count them to get a picture of what's affecting the system health
	Warn(msg string)
	// Error - Definition:
	// something that the app's doing that it shouldn't.
	// This isn't a user error ('invalid search query');
	// it's an assertion failure, network problem, etc etc.,
	// probably one that is going to abort the current operation
	Error(msg string)
	// WrappedError - Definition:
	// this is a convenience method that calls Error() but makes sure to wrap the error a final time
	// so that all current call context is included in the error. This has the same output as:
	//   logger.WithFields(logger.WrapError(err).Fields()).WithError(logger.WrapError(err)).Error("failed to process request")
	// but instead has a much simpler oneliner of:
	//   logger.WrappedError(err, "failed to process request")
	WrappedError(Error, string)
	// Fatal - Definition:
	// the app (or at the very least a thread) is about to die horribly.
	// This is where the info explaining why that's happening goes.
	Fatal(msg string)
	// Panic - Definition:
	// Be careful about calling this vs Fatal:
	// - For Fatal level, the log message goes to the configured log output, while panic is only going to write to stderr.
	// - Panic will print a stack trace, which may not be relevant to the error at all.
	// - Defers will be executed when a program panics, but calling os.Exit exits immediately, and deferred functions can't be run.
	// In general, only use panic for programming errors, where the stack trace is important to the context of the error.
	// If the message isn't targeted at the programmer, you're simply hiding the message in superfluous data.
	Panic(msg string)
}

// logger wraps around logrus and implements the Logger interface
type logger struct {
	entry *logrus.Entry
}

// Fields is used to pass to the WithFields() interface method
type Fields map[string]interface{}

// newLogger creates a Logger with defined logrus Entry
func newLogger(e *logrus.Entry) logger {
	return logger{entry: e}
}

// Set methods

// AddGlobalField adds a key:value pair to every log written with this logger
// This is helpful for service-wide values
func (l *logger) AddGlobalField(key string, val string) {
	l.WithField(key, val).Info("Adding field to global logger")
	l.entry = l.entry.WithField(key, val)
}

// SetLevel sets the logging level for the logger
func (l *logger) SetLevel(level logrus.Level) {
	l.entry.Logger.SetLevel(level)
}

// Get methods

// GetEntry returns a copy of the logrus Entry used in the logger
// NOTE: Only use the base logrus.Entry when absolutely necessary.
// Logging should really be done through the Logger interface,
// NOT through logrus directly.
func (l *logger) GetEntry() *logrus.Entry {
	return l.entry
}

// WrapError wraps a standard Go error (OR `logging.Error`) into a custom error with the
// additional context of current logger fields and call stack information.
func (l *logger) WrapError(err error) Error {
	wrappedError := wrap(err, Fields(l.entry.Data))
	if wrappedError == nil {
		l.entry.Error("failed to wrap error")
		// return custom error but without any wrapping
		return customError{
			cause: err,
		}
	}
	return wrappedError
}

// With methods

// WithError adds an error as single field (using the key defined in ErrorKey) to the Entry.
func (l *logger) WithError(err error) *logger {
	newLogger := newLogger(l.entry.WithError(err))
	return &newLogger
}

// WithContext adds a context to the Entry.
func (l *logger) WithContext(ctx context.Context) *logger {
	newLogger := newLogger(l.entry.WithContext(ctx))
	return &newLogger
}

// WithField adds a single field to the Entry.
func (l *logger) WithField(key string, value interface{}) *logger {
	newLogger := newLogger(l.entry.WithField(key, value))
	return &newLogger
}

// WithFields adds a map of fields to the Entry.
func (l *logger) WithFields(fields Fields) *logger {
	// Copy custom fields into logrus fields
	logrusFields := logrus.Fields{}
	for index, element := range fields {
		logrusFields[index] = element
	}
	// Create new logger with given fields
	newLogger := newLogger(l.entry.WithFields(logrusFields))
	return &newLogger
}

// WithTime overrides the time of the Entry.
func (l *logger) WithTime(t time.Time) *logger {
	newLogger := newLogger(l.entry.WithTime(t))
	return &newLogger
}

// Log methods

func (l *logger) Trace(msg string) {
	l.withLogContext().entry.Trace(msg)
}

func (l *logger) Debug(msg string) {
	l.withLogContext().entry.Debug(msg)
}

func (l *logger) Info(msg string) {
	l.withLogContext().entry.Info(msg)
}

func (l *logger) Warn(msg string) {
	l.withLogContext().entry.Warn(msg)
}

func (l *logger) Error(msg string) {
	l.withLogContext().entry.Error(msg)
}

func (l *logger) WrappedError(err Error, msg string) {
	err = wrap(err, Fields(l.entry.Data))
	l.WithFields(err.Fields()).WithError(err).withLogContext().entry.Error(msg)
}

func (l *logger) Fatal(msg string) {
	l.withLogContext().entry.Fatal(msg)
}

func (l *logger) Panic(msg string) {
	l.withLogContext().entry.Panic(msg)
}

// Helper methods

// withLogContext makes sure that the external logger module uses
// the function name of the calling function instead of here (ex. Debug())
func (l *logger) withLogContext() *logger {
	pc, _, _, ok := runtime.Caller(2)
	if !ok {
		return nil
	}
	fn := runtime.FuncForPC(pc).Name()
	functionName := fn[strings.LastIndex(fn, ".")+1:]
	return l.WithField("function", functionName)
}
