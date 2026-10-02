// Package logging holds the verbose diagnostic log. It is off by default.
// The -v flag turns on debug records; -vv adds trace records. The log goes
// to a file because Bubble Tea owns the terminal.
package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"syscall"
	"time"
)

// LevelTrace is below slog.LevelDebug. It marks very noisy records.
const LevelTrace = slog.Level(-8)

var (
	logger  = slog.New(discardHandler{})
	level   atomic.Int64
	enabled atomic.Bool
	out     io.Writer = io.Discard
)

func init() { level.Store(int64(slog.LevelInfo + 100)) }

type discardHandler struct{}

func (discardHandler) Enabled(context.Context, slog.Level) bool  { return false }
func (discardHandler) Handle(context.Context, slog.Record) error { return nil }
func (d discardHandler) WithAttrs([]slog.Attr) slog.Handler      { return d }
func (d discardHandler) WithGroup(string) slog.Handler           { return d }

// Setup opens path in append mode and routes the log there. verbosity 1
// means debug, 2 or more means trace. A verbosity of 0 leaves logging off.
// The returned function closes the file.
func Setup(path string, verbosity int, header string) (func(), error) {
	if verbosity <= 0 {
		return func() {}, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	lvl := slog.LevelDebug
	if verbosity >= 2 {
		lvl = LevelTrace
	}
	level.Store(int64(lvl))
	out = f
	h := slog.NewTextHandler(f, &slog.HandlerOptions{
		Level:     lvl,
		AddSource: true,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.LevelKey {
				if l, ok := a.Value.Any().(slog.Level); ok && l == LevelTrace {
					a.Value = slog.StringValue("TRACE")
				}
			}
			if a.Key == slog.SourceKey {
				if s, ok := a.Value.Any().(*slog.Source); ok {
					a.Value = slog.StringValue(fmt.Sprintf("%s:%d", filepath.Base(s.File), s.Line))
				}
			}
			return a
		},
	})
	logger = slog.New(h)
	enabled.Store(true)
	logger.Info("log start", "header", header, "pid", os.Getpid(), "level", lvl.String())
	installSignalDump()
	return func() {
		logger.Info("log end")
		enabled.Store(false)
		f.Close()
	}, nil
}

// Enabled reports whether any logging is on.
func Enabled() bool { return enabled.Load() }

// TraceEnabled reports whether trace records are on.
func TraceEnabled() bool { return enabled.Load() && slog.Level(level.Load()) <= LevelTrace }

func log(l slog.Level, msg string, args ...any) {
	if !enabled.Load() || l < slog.Level(level.Load()) {
		return
	}
	var pcs [1]uintptr
	runtime.Callers(3, pcs[:])
	r := slog.NewRecord(time.Now(), l, msg, pcs[0])
	r.Add(args...)
	_ = logger.Handler().Handle(context.Background(), r)
}

// Trace logs a very noisy record (only with -vv).
func Trace(msg string, args ...any) { log(LevelTrace, msg, args...) }

// Debug logs a debug record (with -v or -vv).
func Debug(msg string, args ...any) { log(slog.LevelDebug, msg, args...) }

// Info logs an info record.
func Info(msg string, args ...any) { log(slog.LevelInfo, msg, args...) }

// Warn logs a warning record.
func Warn(msg string, args ...any) { log(slog.LevelWarn, msg, args...) }

// Error logs an error record.
func Error(msg string, args ...any) { log(slog.LevelError, msg, args...) }

// DumpGoroutines writes every goroutine stack to the log with a reason.
func DumpGoroutines(reason string) {
	if !enabled.Load() {
		return
	}
	buf := make([]byte, 1<<20)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			buf = buf[:n]
			break
		}
		buf = make([]byte, len(buf)*2)
	}
	logger.Warn("goroutine dump", "reason", reason, "goroutines", runtime.NumGoroutine())
	fmt.Fprintf(out, "----- goroutine dump begin (%s) -----\n%s\n----- goroutine dump end -----\n", reason, buf)
}

func installSignalDump() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGUSR1)
	go func() {
		for range ch {
			DumpGoroutines("SIGUSR1")
		}
	}()
}

// Watchdog tracks the last time the interface loop made progress. When the
// loop stalls longer than limit, it dumps all goroutines once per stall.
type Watchdog struct {
	last  atomic.Int64
	busy  atomic.Value // string: what the loop handles now
	limit time.Duration
}

// StartWatchdog starts a watchdog. It returns nil when logging is off.
func StartWatchdog(limit time.Duration) *Watchdog {
	if !enabled.Load() {
		return nil
	}
	w := &Watchdog{limit: limit}
	w.Beat("start")
	go w.run()
	return w
}

// Beat records progress. what names the work the loop handles now.
func (w *Watchdog) Beat(what string) {
	if w == nil {
		return
	}
	w.last.Store(time.Now().UnixNano())
	w.busy.Store(what)
}

func (w *Watchdog) run() {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	dumped := false
	for range t.C {
		if !enabled.Load() {
			return
		}
		since := time.Since(time.Unix(0, w.last.Load()))
		if since > w.limit {
			if !dumped {
				what, _ := w.busy.Load().(string)
				Error("watchdog: interface loop stalled", "since", since.Round(time.Millisecond), "last", what)
				DumpGoroutines("watchdog stall")
				dumped = true
			}
		} else if dumped {
			Warn("watchdog: interface loop recovered", "stalled_for", since)
			dumped = false
		}
	}
}
