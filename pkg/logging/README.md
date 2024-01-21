# logging

## Overview

A central logging module that acts as a custom wrapper around logrus. It provides definitions of logging levels, trace injection, and global logger creation and level setting.

## Understanding log levels

Each log level in the custom logger here also has a preceding comment explaining how it is defined. Make sure to know and follow these definitions so that log levels are consistent across all microservices.

For example:

```golang
// Warn - Definition:
// something that's concerning but not causing the operation to abort;
// # of connections in the DB pool getting low, an unusual-but-expected timeout in an operation, etc.
// Think of 'WARN' as something that's useful in aggregate; e.g. grep, group,
// and count them to get a picture of what's affecting the system health
func (l *CustomLogger) Warn(args ...interface{}) {
    l.entry.Warn(args...)
}
```

## Usage

Implementing the logger is very simple. The main thing to keep in mind is that this logger should **ALWAYS** be used as a global logger passed down from `main.go` into the handler layer *at instantiation*, and then passed down from each handler function into the service and dao layers *at call time*. This is to ensure that log level, traces, and other fields are properly preserved in all logs.
