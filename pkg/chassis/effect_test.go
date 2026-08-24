package chassis

import (
	"context"
	"errors"
	"testing"
)

// fatalCalled is the panic value fakeLogger.Fatal raises so tests can detect a Fatal
// call without the test process actually exiting the way a real Logger's Fatal does.
type fatalCalled struct{ msg string }

// fakeLogger is a minimal Logger implementation for tests. All methods are no-ops
// except Fatal, which panics with fatalCalled instead of exiting the process.
type fakeLogger struct{}

func newFakeLogger() *fakeLogger { return &fakeLogger{} }

func (l *fakeLogger) Start(Config)                       {}
func (l *fakeLogger) SetLevel(LogLevel)                  {}
func (l *fakeLogger) GetLevel() LogLevel                 { return InfoLevel }
func (l *fakeLogger) Wrap(err error) error               { return err }
func (l *fakeLogger) WithError(error) Logger             { return l }
func (l *fakeLogger) WithContext(context.Context) Logger { return l }
func (l *fakeLogger) WithField(string, any) Logger       { return l }
func (l *fakeLogger) WithFields(Fields) Logger           { return l }
func (l *fakeLogger) WithCallDepth(int) Logger           { return l }
func (l *fakeLogger) Trace(string)                       {}
func (l *fakeLogger) Debug(string)                       {}
func (l *fakeLogger) Debugf(string, ...any)              {}
func (l *fakeLogger) Info(string)                        {}
func (l *fakeLogger) Infof(string, ...any)               {}
func (l *fakeLogger) Warn(string)                        {}
func (l *fakeLogger) Warnf(string, ...any)               {}
func (l *fakeLogger) Error(string)                       {}
func (l *fakeLogger) Errorf(string, ...any)              {}
func (l *fakeLogger) WrappedError(error, string)         {}
func (l *fakeLogger) Fatal(msg string)                   { panic(fatalCalled{msg: msg}) }
func (l *fakeLogger) Panic(msg string)                   { panic(msg) }

func newTestRuntime() *Runtime {
	return &Runtime{logger: newFakeLogger()}
}

func TestEffect_SetupOnceDisposeOnce(t *testing.T) {
	rt := newTestRuntime()
	setupCalls, disposeCalls := 0, 0

	rt.Effect("test", func() (func(context.Context) error, error) {
		setupCalls++
		return func(context.Context) error {
			disposeCalls++
			return nil
		}, nil
	})

	if setupCalls != 1 {
		t.Fatalf("setup called %d times, want 1", setupCalls)
	}
	if disposeCalls != 0 {
		t.Fatalf("dispose called %d times before shutdown, want 0", disposeCalls)
	}

	rt.shutdown()

	if disposeCalls != 1 {
		t.Fatalf("dispose called %d times after shutdown, want 1", disposeCalls)
	}

	// shutdown is guarded by sync.Once — a second call must not dispose again.
	rt.shutdown()
	if disposeCalls != 1 {
		t.Fatalf("dispose called %d times after second shutdown, want 1", disposeCalls)
	}
}

func TestEffect_LIFOOrder(t *testing.T) {
	rt := newTestRuntime()
	var order []string

	for _, name := range []string{"first", "second", "third"} {
		rt.Effect(name, func() (func(context.Context) error, error) {
			return func(context.Context) error {
				order = append(order, name)
				return nil
			}, nil
		})
	}

	rt.shutdown()

	want := []string{"third", "second", "first"}
	if len(order) != len(want) {
		t.Fatalf("dispose order %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("dispose order %v, want %v", order, want)
		}
	}
}

func TestEffect_NilDisposeIsSkipped(t *testing.T) {
	rt := newTestRuntime()

	rt.Effect("no-op", func() (func(context.Context) error, error) {
		return nil, nil
	})

	if len(rt.effects) != 0 {
		t.Fatalf("effect stack has %d entries, want 0 for a nil dispose", len(rt.effects))
	}

	// must not panic
	rt.shutdown()
}

func TestEffect_SetupErrorTriggersFatalAndRegistersNothing(t *testing.T) {
	rt := newTestRuntime()

	func() {
		defer func() {
			r := recover()
			if _, ok := r.(fatalCalled); !ok {
				t.Fatalf("expected Effect to call Logger.Fatal on setup error, got panic value %#v", r)
			}
		}()
		rt.Effect("boom", func() (func(context.Context) error, error) {
			return nil, errors.New("setup failed")
		})
		t.Fatal("expected a panic from Fatal, but Effect returned normally")
	}()

	if len(rt.effects) != 0 {
		t.Fatalf("effect stack has %d entries after a failed setup, want 0 — nothing should be tracked for a setup that never completed", len(rt.effects))
	}
}

func TestEffect_DisposeErrorDoesNotStopRemainingEffects(t *testing.T) {
	rt := newTestRuntime()
	var disposed []string

	rt.Effect("ok-1", func() (func(context.Context) error, error) {
		return func(context.Context) error {
			disposed = append(disposed, "ok-1")
			return nil
		}, nil
	})
	rt.Effect("failing", func() (func(context.Context) error, error) {
		return func(context.Context) error {
			disposed = append(disposed, "failing")
			return errors.New("dispose failed")
		}, nil
	})
	rt.Effect("ok-2", func() (func(context.Context) error, error) {
		return func(context.Context) error {
			disposed = append(disposed, "ok-2")
			return nil
		}, nil
	})

	rt.shutdown()

	want := []string{"ok-2", "failing", "ok-1"}
	if len(disposed) != len(want) {
		t.Fatalf("disposed %v, want %v", disposed, want)
	}
	for i := range want {
		if disposed[i] != want[i] {
			t.Fatalf("disposed %v, want %v", disposed, want)
		}
	}
}
