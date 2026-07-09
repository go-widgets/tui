// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

//go:build unix

package tui

import (
	"io"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// App is an interactive TUI runner. Instantiate one, set [App.Root]
// to your widget tree, optionally register keybindings + a tick rate,
// and call [App.Run]. The App handles alt-screen + raw mode, parses
// stdin into [toolkit.Event] values, dispatches them to the widget
// tree, repaints on demand, and cleans up on exit — including on
// panic, via a deferred TTY.Leave.
type App struct {
	// Root is the widget tree the App draws and dispatches events
	// to. Callers wire the composition once and mutate its fields
	// during OnKey callbacks; the next repaint reflects the state.
	Root toolkit.Widget

	// Theme cascades through the widget tree on every draw. Defaults
	// to [toolkit.DefaultLight] if left nil.
	Theme *toolkit.Theme

	// Keys is a map from a key Code (matching InputParser output —
	// "q", "Ctrl+C", "Up", "Enter", …) to a handler. Handlers can
	// mutate App state (e.g. call [App.Quit]) or the widget tree.
	// Global handlers run BEFORE the event reaches Root.OnEvent — the
	// App always dispatches to Root afterwards.
	Keys map[string]func(*App)

	// InputTarget, when non-nil, receives every event that a Keys handler
	// does not Consume — INSTEAD of Root. It is a modal input capture: a
	// command palette or search box sets it to swallow typing while open,
	// then clears it (sets nil) to hand input back to Root. Root is still
	// drawn every frame; only event routing changes.
	InputTarget toolkit.Widget

	// TickHz sets the auto-tick frequency in Hz. 0 disables ticks. A
	// widget subscribing to EventTick needs TickHz > 0 to animate.
	TickHz int

	// runtime state — reset each Run.
	stdin    io.Reader
	stdout   io.Writer
	tty      TTY
	parser   *InputParser
	quit     chan struct{}
	quitOnce sync.Once
	wakeCh   chan struct{}
	dirty    bool
	cols     int
	rows     int
	exitCode int

	// consumed is set by [App.Consume] from inside a Keys handler to
	// signal that the current event should NOT propagate to Root.
	// The event loop resets it to false at the top of every event
	// dispatch so a stale value from a previous event never leaks
	// forward.
	consumed bool

	// seams — swapped by tests to bypass every OS-touching call.
	openTTYFn func(*os.File) (TTY, error)
	stdinFn   func() io.Reader
	stdoutFn  func() *os.File
	signalFn  func() (resize <-chan struct{}, interrupt <-chan struct{}, stop func())
	tickFn    func(time.Duration) (<-chan time.Time, func())
}

// NewApp returns an App wired to the real terminal: OpenTTY, os.Stdin,
// os.Stdout, real Unix signal handling, and time.NewTicker for the
// tick channel. Tests build an App by hand and swap the seams instead.
func NewApp() *App {
	return &App{
		Keys:      map[string]func(*App){},
		Theme:     toolkit.DefaultLight(),
		quit:      make(chan struct{}),
		openTTYFn: OpenTTY,
		stdinFn:   func() io.Reader { return os.Stdin },
		stdoutFn:  func() *os.File { return os.Stdout },
		signalFn:  realSignalFn,
		tickFn:    realTickFn,
	}
}

// realSignalFn wires SIGWINCH → resize and os.Interrupt → interrupt,
// so a terminal resize or a Ctrl+C reaches the event loop. The
// returned stop func unregisters both handlers and terminates the
// forwarder goroutine.
func realSignalFn() (resize <-chan struct{}, interrupt <-chan struct{}, stop func()) {
	resizeCh := make(chan struct{}, 1)
	interruptCh := make(chan struct{}, 1)
	winchSig := make(chan os.Signal, 1)
	intSig := make(chan os.Signal, 1)
	signal.Notify(winchSig, syscall.SIGWINCH)
	signal.Notify(intSig, os.Interrupt)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-winchSig:
				select {
				case resizeCh <- struct{}{}:
				default:
				}
			case <-intSig:
				select {
				case interruptCh <- struct{}{}:
				default:
				}
			case <-done:
				return
			}
		}
	}()
	return resizeCh, interruptCh, func() {
		signal.Stop(winchSig)
		signal.Stop(intSig)
		close(done)
	}
}

// realTickFn wraps time.NewTicker so the App's tick seam matches the
// test seam's (channel, stop) shape without every caller poking at
// ticker internals.
func realTickFn(d time.Duration) (<-chan time.Time, func()) {
	t := time.NewTicker(d)
	return t.C, t.Stop
}

// Quit signals the event loop to exit on the next iteration. Safe to
// call from a key handler or a goroutine, and idempotent: a second
// call is a no-op (the underlying quit channel is closed exactly once
// via sync.Once).
func (a *App) Quit() {
	a.quitOnce.Do(func() { close(a.quit) })
}

// IsQuitting reports whether Quit has been called (whether the loop is
// on its way to exit). Non-blocking. Useful for consumer tests that
// verify a key handler triggered a quit without spinning the event
// loop.
func (a *App) IsQuitting() bool {
	select {
	case <-a.quit:
		return true
	default:
		return false
	}
}

// SetOpenTTYFn overrides the TTY factory the event loop uses at Run
// time. Consumer tests inject a fake TTY (or an error) so they can
// drive Run() without needing a real controlling terminal. Passing
// a factory that returns an error is a valid way to force Run() to
// exit with a non-zero code before any raw-mode side effect.
func (a *App) SetOpenTTYFn(fn func(*os.File) (TTY, error)) {
	a.openTTYFn = fn
}

// Consume tells the event loop that the current Keys handler fully
// handled the event and it must NOT propagate to Root.OnEvent.
// Without Consume, every key that matches a Keys handler also
// reaches Root — which is fine for global shortcuts (Ctrl+C to quit
// still lets the Root repaint from its own state), but wrong for
// mode-switching editors where pressing 'i' to enter edit mode
// would otherwise ALSO insert 'i' into the underlying TextView.
//
// Idempotent within a single event dispatch. The event loop clears
// the flag at the top of every event so a Consume from a previous
// event never affects the next one.
func (a *App) Consume() { a.consumed = true }

// Refresh marks the frame dirty so the next iteration repaints. It
// also nudges the wake channel so a loop currently blocked in select
// unblocks — enabling async I/O goroutines to schedule a redraw
// without racing an unrelated event.
func (a *App) Refresh() {
	a.dirty = true
	if a.wakeCh != nil {
		select {
		case a.wakeCh <- struct{}{}:
		default:
		}
	}
}

// Run blocks, driving the event loop until Quit is called or an
// interrupt signal arrives. Returns the exit code (0 by default; 1 if
// TTY setup fails; 2 if a key handler or widget panics). Terminal
// state is guaranteed to be restored on exit — even on panic — via a
// deferred TTY.Leave.
//
// The return is named (exitCode) so a panic recovered by the deferred
// recover func can overwrite it; without the named return the panic
// path would still yield the pre-panic value.
func (a *App) Run() (exitCode int) {
	a.quit = make(chan struct{})
	a.quitOnce = sync.Once{}
	a.wakeCh = make(chan struct{}, 1)
	a.exitCode = 0

	stdoutFile := a.stdoutFn()
	a.stdout = stdoutFile
	tty, err := a.openTTYFn(stdoutFile)
	if err != nil {
		a.exitCode = 1
		return a.exitCode
	}
	a.tty = tty
	defer func() { _ = tty.Leave() }()

	// Panic recovery is registered AFTER the Leave defer so it runs
	// FIRST on unwind (LIFO). recover() then swallows the panic and
	// sets the named return; the Leave defer still fires to restore
	// terminal state.
	defer func() {
		if r := recover(); r != nil {
			a.exitCode = 2
			exitCode = 2
		}
	}()

	if err := tty.Enter(); err != nil {
		a.exitCode = 1
		return a.exitCode
	}

	resizeCh, interruptCh, stopSig := a.signalFn()
	defer stopSig()

	var tickCh <-chan time.Time
	stopTick := func() {}
	if a.TickHz > 0 {
		d := time.Second / time.Duration(a.TickHz)
		tickCh, stopTick = a.tickFn(d)
	}
	defer stopTick()

	a.parser = NewInputParser()
	a.stdin = a.stdinFn()

	inCh := make(chan []byte, 4)
	go func() {
		buf := make([]byte, 128)
		for {
			n, err := a.stdin.Read(buf)
			if n > 0 {
				b := make([]byte, n)
				copy(b, buf[:n])
				select {
				case inCh <- b:
				case <-a.quit:
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	a.refreshSize()
	a.draw()

loop:
	for {
		select {
		case b := <-inCh:
			for _, ev := range a.parser.Feed(b) {
				a.consumed = false
				if h, ok := a.Keys[ev.Code]; ok {
					h(a)
				}
				if !a.consumed {
					// Modal capture wins over Root when set.
					if a.InputTarget != nil {
						a.InputTarget.OnEvent(ev)
					} else if a.Root != nil {
						a.Root.OnEvent(ev)
					}
				}
			}
			a.dirty = true
		case <-resizeCh:
			a.refreshSize()
			a.dirty = true
		case <-interruptCh:
			a.Quit()
		case <-tickCh:
			if a.Root != nil {
				a.Root.OnEvent(toolkit.Event{Kind: EventTick})
			}
			a.dirty = true
		case <-a.wakeCh:
			// woken by Refresh from outside — dirty already set.
		case <-a.quit:
			break loop
		}
		if a.dirty {
			a.draw()
			a.dirty = false
		}
	}
	return a.exitCode
}

// refreshSize re-queries the TTY dimensions after resize (and on
// startup) and propagates the new bounds to Root. A Size() error, or
// a non-positive value, falls back to [DefaultCols] / [DefaultRows]
// so the App never renders into a zero-sized grid.
func (a *App) refreshSize() {
	cols, rows, err := a.tty.Size()
	if err != nil || cols <= 0 || rows <= 0 {
		cols, rows = DefaultCols, DefaultRows
	}
	a.cols, a.rows = cols, rows
	if a.Root != nil {
		a.Root.SetBounds(toolkit.Rect{X: 0, Y: 0, W: cols, H: rows})
	}
}

// draw renders one frame to a.stdout: home the cursor so the frame
// overwrites the previous one in-place (alt-screen doesn't auto-
// scroll), paint the background, delegate to Root.Draw, and flush
// the resulting ANSI stream. cols/rows fall back to defaults on a
// nil / zero size so a Draw call never underflows the painter.
func (a *App) draw() {
	cols, rows := a.cols, a.rows
	if cols <= 0 || rows <= 0 {
		cols, rows = DefaultCols, DefaultRows
	}
	theme := a.Theme
	if theme == nil {
		theme = toolkit.DefaultLight()
	}
	_, _ = a.stdout.Write([]byte("\x1b[H"))
	cp := painter.NewCellPainter(cols, rows)
	cp.FillRect(painter.Rect{X: 0, Y: 0, W: cols, H: rows}, toPainterRGBA(theme.Background))
	if a.Root != nil {
		a.Root.Draw(cp, theme)
	}
	_, _ = cp.WriteANSI(a.stdout)
}
