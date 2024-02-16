package chassis

import (
	"context"
	"strings"
)

// Fields is an alias primarily used for Logger methods
type Fields map[string]any

// LogLevel type
type LogLevel uint32

const (
	// PanicLevel level, highest level of severity. Logs and then calls panic with the
	// message passed to Debug, Info, ...
	PanicLevel LogLevel = iota
	// FatalLevel level. Logs and then calls `os.Exit(1)`. It will exit even if the
	// logging level is set to Panic.
	FatalLevel
	// ErrorLevel level. Logs. Used for errors that should definitely be noted.
	// Commonly used for hooks to send errors to an error tracking service.
	ErrorLevel
	// WarnLevel level. Non-critical entries that deserve eyes.
	WarnLevel
	// InfoLevel level. General operational entries about what's going on inside the
	// application.
	InfoLevel
	// DebugLevel level. Usually only enabled when debugging. Very verbose logging.
	DebugLevel
	// TraceLevel level. Designates finer-grained informational events than the Debug.
	TraceLevel
)

type Logger interface {
	// Start configures the logger for service startup
	Start(config Config)
	// SetLevel sets the logging level for the logger
	SetLevel(level LogLevel)
	// GetLevel gets the current logging level for the logger
	GetLevel() LogLevel
	// Wrap wraps an error with the additional context of current
	// logger fields and call stack information.
	Wrap(error) error
	// WithError - Add an error as single field (using the key defined in ErrorKey) to the logger.
	WithError(err error) Logger
	// WithContext - Add a context to the logger.
	WithContext(ctx context.Context) Logger
	// WithField - Add a single field to the logger.
	WithField(key string, value any) Logger
	// WithFields - Add a map of fields to the logger.
	WithFields(fields Fields) Logger
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
	WrappedError(error, string)
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

// ParseLogLevel takes a string level and returns the log level constant.
func ParseLogLevel(level string) LogLevel {
	switch strings.ToLower(level) {
	case "panic":
		return PanicLevel
	case "fatal":
		return FatalLevel
	case "error":
		return ErrorLevel
	case "warn", "warning":
		return WarnLevel
	case "info":
		return InfoLevel
	case "debug":
		return DebugLevel
	case "trace":
		return TraceLevel
	default:
		return InfoLevel
	}
}

func (l LogLevel) String() string {
	switch l {
	case PanicLevel:
		return "panic"
	case FatalLevel:
		return "fatal"
	case ErrorLevel:
		return "error"
	case WarnLevel:
		return "warn"
	case InfoLevel:
		return "info"
	case DebugLevel:
		return "debug"
	case TraceLevel:
		return "trace"
	default:
		return "unknown"
	}
}
