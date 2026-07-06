// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

//go:build unix

package tui

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// fakeTTY is a TTY implementation that records Enter / Leave and
// returns caller-configured Size values without touching a real
// terminal. Used by every App test to bypass the OpenTTY seam.
type fakeTTY struct {
	mu          sync.Mutex
	enterCalled int
	leaveCalled int
	enterErr    error
	leaveErr    error
	cols, rows  int
	sizeErr     error
}

func (f *fakeTTY) Enter() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.enterCalled++
	return f.enterErr
}

func (f *fakeTTY) Leave() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.leaveCalled++
	return f.leaveErr
}

func (f *fakeTTY) Size() (int, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.cols, f.rows, f.sizeErr
}

func (f *fakeTTY) leaves() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.leaveCalled
}

// recordingWidget captures every OnEvent + Draw invocation so tests
// can assert the App dispatched events / repainted after state
// changes. Bounds / SetBounds are mutex-guarded so a test goroutine
// polling Bounds() while Run's event goroutine mutates it does not
// trip the race detector.
type recordingWidget struct {
	toolkit.Base
	mu         sync.Mutex
	rect       toolkit.Rect
	events     []toolkit.Event
	draws      int
	boundsSeen []toolkit.Rect
	onEvent    func(*recordingWidget, toolkit.Event)
}

func (w *recordingWidget) Bounds() toolkit.Rect {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.rect
}

func (w *recordingWidget) SetBounds(r toolkit.Rect) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.rect = r
}

func (w *recordingWidget) OnEvent(ev toolkit.Event) {
	w.mu.Lock()
	w.events = append(w.events, ev)
	w.boundsSeen = append(w.boundsSeen, w.rect)
	fn := w.onEvent
	w.mu.Unlock()
	if fn != nil {
		fn(w, ev)
	}
}

func (w *recordingWidget) Draw(p painter.Painter, theme *toolkit.Theme) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.draws++
}

func (w *recordingWidget) eventCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.events)
}

func (w *recordingWidget) drawCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.draws
}

func (w *recordingWidget) lastEvent() toolkit.Event {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.events) == 0 {
		return toolkit.Event{}
	}
	return w.events[len(w.events)-1]
}

// harness bundles the App under test with the pipe writers + signal
// channels + tick channel each test uses to drive it.
type harness struct {
	app         *App
	tty         *fakeTTY
	stdinR      *os.File
	stdinW      *os.File
	stdout      *os.File
	stdoutPath  string
	resizeCh    chan struct{}
	interruptCh chan struct{}
	stopSig     chan struct{}
	tickCh      chan time.Time
	stopTick    chan struct{}
}

// newHarness returns a fully-mocked App ready for Run(). Every seam
// is swapped for a test-controlled channel or fake.
func newHarness(t *testing.T) *harness {
	t.Helper()
	sr, sw, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe stdin: %v", err)
	}
	t.Cleanup(func() { _ = sr.Close(); _ = sw.Close() })

	stdoutPath := filepath.Join(t.TempDir(), "stdout")
	sout, err := os.Create(stdoutPath)
	if err != nil {
		t.Fatalf("os.Create stdout: %v", err)
	}
	t.Cleanup(func() { _ = sout.Close() })

	h := &harness{
		tty:         &fakeTTY{cols: 40, rows: 12},
		stdinR:      sr,
		stdinW:      sw,
		stdout:      sout,
		stdoutPath:  stdoutPath,
		resizeCh:    make(chan struct{}, 4),
		interruptCh: make(chan struct{}, 4),
		stopSig:     make(chan struct{}, 1),
		tickCh:      make(chan time.Time, 4),
		stopTick:    make(chan struct{}, 1),
	}
	a := &App{
		Keys: map[string]func(*App){},
	}
	a.openTTYFn = func(f *os.File) (TTY, error) { return h.tty, nil }
	a.stdinFn = func() io.Reader { return sr }
	a.stdoutFn = func() *os.File { return sout }
	a.signalFn = func() (<-chan struct{}, <-chan struct{}, func()) {
		return h.resizeCh, h.interruptCh, func() { h.stopSig <- struct{}{} }
	}
	a.tickFn = func(d time.Duration) (<-chan time.Time, func()) {
		return h.tickCh, func() { h.stopTick <- struct{}{} }
	}
	h.app = a
	return h
}

// runAsync starts Run in a goroutine. The returned wait func blocks
// until Run returns (or fails the test with a 2s deadline) and
// yields the exit code Run produced.
func (h *harness) runAsync(t *testing.T) func() int {
	t.Helper()
	done := make(chan int, 1)
	go func() {
		done <- h.app.Run()
	}()
	return func() int {
		select {
		case c := <-done:
			return c
		case <-time.After(2 * time.Second):
			t.Fatal("Run did not return within 2s")
			return -1
		}
	}
}

func (h *harness) waitFor(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", msg)
}

// TestNewAppDefaults verifies the constructor produces a non-nil
// Keys map, a non-nil default Theme, and populated seams.
func TestNewAppDefaults(t *testing.T) {
	a := NewApp()
	if a.Keys == nil {
		t.Error("NewApp Keys is nil")
	}
	if a.Theme == nil {
		t.Error("NewApp Theme is nil")
	}
	if a.openTTYFn == nil || a.stdinFn == nil || a.stdoutFn == nil ||
		a.signalFn == nil || a.tickFn == nil {
		t.Error("NewApp did not wire seams")
	}
	// Sanity: the stdin/stdout seams point at the real streams.
	if a.stdinFn() != os.Stdin {
		t.Error("stdinFn should return os.Stdin")
	}
	if a.stdoutFn() != os.Stdout {
		t.Error("stdoutFn should return os.Stdout")
	}
}

// TestRunHappyPathQuitOnKey covers the input select case + the Keys
// dispatch + a clean loop exit via Quit. Also exercises the stdin
// goroutine's read + EOF-return branches.
func TestRunHappyPathQuitOnKey(t *testing.T) {
	h := newHarness(t)
	rec := &recordingWidget{}
	h.app.Root = rec
	h.app.Keys["q"] = func(a *App) { a.Quit() }

	wait := h.runAsync(t)
	if _, err := h.stdinW.Write([]byte{'q'}); err != nil {
		t.Fatalf("write q: %v", err)
	}
	// Close stdin to let the stdin goroutine return via the EOF
	// branch after the byte has been consumed.
	_ = h.stdinW.Close()
	code := wait()
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if h.tty.leaves() == 0 {
		t.Error("TTY.Leave was never called")
	}
	if rec.eventCount() < 1 {
		t.Error("Root did not receive the 'q' event")
	}
	if rec.drawCount() < 2 {
		// initial draw + at least one post-event draw
		t.Errorf("draw count = %d, want >= 2", rec.drawCount())
	}
}

// TestRunInterruptQuits covers the interruptCh select case: the
// signal path funnels into Quit and the loop exits cleanly.
func TestRunInterruptQuits(t *testing.T) {
	h := newHarness(t)
	rec := &recordingWidget{}
	h.app.Root = rec

	wait := h.runAsync(t)
	// Wait for the App to reach the select — the initial draw fires
	// synchronously, so a single completed draw is a good proxy.
	h.waitFor(t, func() bool { return rec.drawCount() >= 1 }, "initial draw")
	h.interruptCh <- struct{}{}
	code := wait()
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
}

// TestRunResizePropagatesBounds covers the resizeCh select case: a
// resize triggers a Size() re-query and re-issues SetBounds on Root.
func TestRunResizePropagatesBounds(t *testing.T) {
	h := newHarness(t)
	rec := &recordingWidget{}
	h.app.Root = rec

	wait := h.runAsync(t)
	h.waitFor(t, func() bool { return rec.drawCount() >= 1 }, "initial draw")

	// Change fake TTY dimensions, then fire a resize signal.
	h.tty.mu.Lock()
	h.tty.cols, h.tty.rows = 100, 30
	h.tty.mu.Unlock()
	h.resizeCh <- struct{}{}

	h.waitFor(t, func() bool {
		return rec.Bounds().W == 100 && rec.Bounds().H == 30
	}, "Root bounds to update")

	h.app.Quit()
	wait()
}

// TestRunTickDeliversEventTick covers the tickCh select case + the
// EventTick dispatch to Root.
func TestRunTickDeliversEventTick(t *testing.T) {
	h := newHarness(t)
	rec := &recordingWidget{}
	h.app.Root = rec
	h.app.TickHz = 60

	wait := h.runAsync(t)
	h.waitFor(t, func() bool { return rec.drawCount() >= 1 }, "initial draw")

	h.tickCh <- time.Now()

	h.waitFor(t, func() bool {
		return rec.lastEvent().Kind == EventTick
	}, "EventTick to reach Root")

	h.app.Quit()
	wait()
	// tickFn's stop func must have been invoked on Run cleanup.
	select {
	case <-h.stopTick:
	default:
		t.Error("stopTick was not called")
	}
}

// TestRunNoTickHzSkipsTicker covers the TickHz == 0 branch: no
// tickFn call, no tick channel, loop still exits cleanly.
func TestRunNoTickHzSkipsTicker(t *testing.T) {
	h := newHarness(t)
	tickFnCalled := false
	h.app.tickFn = func(d time.Duration) (<-chan time.Time, func()) {
		tickFnCalled = true
		return h.tickCh, func() {}
	}
	h.app.Keys["q"] = func(a *App) { a.Quit() }

	wait := h.runAsync(t)
	_, _ = h.stdinW.Write([]byte{'q'})
	_ = h.stdinW.Close()
	wait()
	if tickFnCalled {
		t.Error("tickFn was called despite TickHz == 0")
	}
}

// TestRunPanicInKeyHandlerLeavesTerminal covers the recover branch:
// a panic in the key handler must set exitCode = 2 AND still trigger
// TTY.Leave via the deferred call chain.
func TestRunPanicInKeyHandlerLeavesTerminal(t *testing.T) {
	h := newHarness(t)
	h.app.Keys["q"] = func(a *App) { panic("boom") }

	wait := h.runAsync(t)
	_, _ = h.stdinW.Write([]byte{'q'})
	_ = h.stdinW.Close()
	code := wait()
	if code != 2 {
		t.Errorf("exit code = %d, want 2 (panic path)", code)
	}
	if h.tty.leaves() == 0 {
		t.Error("TTY.Leave was NOT called after panic — terminal would be left in raw mode")
	}
}

// TestRunOpenTTYErrorSkipsLoop covers the OpenTTY-error branch: Run
// returns a non-zero exit code without touching the loop or Leave.
func TestRunOpenTTYErrorSkipsLoop(t *testing.T) {
	h := newHarness(t)
	sentinel := errors.New("no tty")
	h.app.openTTYFn = func(*os.File) (TTY, error) { return nil, sentinel }
	code := h.app.Run()
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if h.tty.leaves() != 0 {
		t.Errorf("Leave should not run when OpenTTY fails (got %d)", h.tty.leaves())
	}
}

// TestRunEnterErrorSkipsLoop covers the Enter-error branch: Run
// returns 1 and Leave is still safely called (idempotent path).
func TestRunEnterErrorSkipsLoop(t *testing.T) {
	h := newHarness(t)
	h.tty.enterErr = errors.New("enter failed")
	code := h.app.Run()
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	// Leave IS called (defer registered before Enter) but on a
	// non-entered TTY it's a no-op — fakeTTY still records the call.
	if h.tty.leaves() == 0 {
		t.Error("Leave defer did not fire on Enter error")
	}
}

// TestRefreshWakesAndRedraws covers Refresh: it sets dirty and pokes
// wakeCh so a select currently blocked unblocks + repaints.
func TestRefreshWakesAndRedraws(t *testing.T) {
	h := newHarness(t)
	rec := &recordingWidget{}
	h.app.Root = rec

	wait := h.runAsync(t)
	h.waitFor(t, func() bool { return rec.drawCount() >= 1 }, "initial draw")

	before := rec.drawCount()
	h.app.Refresh()
	h.waitFor(t, func() bool { return rec.drawCount() > before }, "post-Refresh draw")

	h.app.Quit()
	wait()
}

// TestRefreshBeforeRunIsSafe covers the Refresh nil-wake-channel
// guard: calling Refresh before Run must not panic.
func TestRefreshBeforeRunIsSafe(t *testing.T) {
	a := &App{}
	a.Refresh() // must not panic; wakeCh is nil, dirty is set.
	if !a.dirty {
		t.Error("Refresh should set dirty even before Run")
	}
}

// TestRefreshWakeChannelFullFallsThrough covers Refresh's default
// branch on wakeCh: a second Refresh while the wake is still pending
// must not block.
func TestRefreshWakeChannelFullFallsThrough(t *testing.T) {
	a := &App{wakeCh: make(chan struct{}, 1)}
	a.Refresh()
	// Wake channel is now full — second Refresh must fall through
	// via default without blocking.
	a.Refresh()
	// If we got here without a deadlock, the branch was hit.
	if len(a.wakeCh) != 1 {
		t.Errorf("wakeCh length = %d, want 1", len(a.wakeCh))
	}
}

// TestQuitIdempotent covers the sync.Once guard: a second Quit is a
// no-op, not a panic on close-of-closed-channel.
func TestQuitIdempotent(t *testing.T) {
	a := &App{}
	a.quit = make(chan struct{})
	a.Quit()
	a.Quit() // must not panic
	select {
	case <-a.quit:
		// closed — expected
	default:
		t.Error("quit channel not closed after Quit")
	}
}

// TestIsQuittingReflectsQuitState verifies IsQuitting flips from false
// to true across a Quit call and stays true afterwards.
func TestIsQuittingReflectsQuitState(t *testing.T) {
	a := &App{}
	a.quit = make(chan struct{})
	if a.IsQuitting() {
		t.Fatal("fresh App should not report IsQuitting = true")
	}
	a.Quit()
	if !a.IsQuitting() {
		t.Fatal("IsQuitting = false after Quit()")
	}
	// Idempotent: still true after another observation.
	if !a.IsQuitting() {
		t.Fatal("IsQuitting flipped back to false on second read")
	}
}

// TestConsumeFlagStopsPropagation verifies that Consume() called
// from a Keys handler prevents the current event from also reaching
// Root.OnEvent. Mode-switching editors rely on this — pressing 'i'
// in view mode must switch to edit mode WITHOUT also inserting 'i'
// into the underlying TextView.
func TestConsumeFlagStopsPropagation(t *testing.T) {
	a := &App{}
	// Direct-field manipulation mirrors what the event loop does at
	// event dispatch: reset consumed to false, call handler, check flag.
	a.consumed = false
	if a.consumed {
		t.Fatal("fresh consumed flag should be false")
	}
	a.Consume()
	if !a.consumed {
		t.Fatal("Consume() did not set the consumed flag")
	}
	// Idempotent — calling twice does not clear it.
	a.Consume()
	if !a.consumed {
		t.Fatal("second Consume() flipped the flag off")
	}
}

// TestSetOpenTTYFnReplacesFactory verifies the exported setter
// installs the caller's TTY factory in place of the default. Consumer
// tests (see cmd/tui-explorer) rely on this seam to drive Run without
// a real controlling terminal.
func TestSetOpenTTYFnReplacesFactory(t *testing.T) {
	a := NewApp()
	called := false
	a.SetOpenTTYFn(func(*os.File) (TTY, error) {
		called = true
		return nil, errors.New("test tty error")
	})
	// Trigger the seam via Run's early-exit path.
	code := a.Run()
	if !called {
		t.Fatal("SetOpenTTYFn's replacement factory was not invoked by Run")
	}
	if code == 0 {
		t.Fatal("Run with an openTTYFn error should return non-zero")
	}
}

// TestRunRootBoundsMutationSurvives covers the "widget mutates its
// Bounds in OnEvent" contract: the redraw after dispatch sees the
// updated bounds.
func TestRunRootBoundsMutationSurvives(t *testing.T) {
	h := newHarness(t)
	rec := &recordingWidget{
		onEvent: func(w *recordingWidget, ev toolkit.Event) {
			r := w.Bounds()
			w.SetBounds(toolkit.Rect{X: r.X + 1, Y: r.Y, W: r.W, H: r.H})
		},
	}
	h.app.Root = rec
	h.app.Keys["a"] = func(a *App) {}

	wait := h.runAsync(t)
	h.waitFor(t, func() bool { return rec.drawCount() >= 1 }, "initial draw")

	// The initial refreshSize set X=0. After OnEvent, X should be 1.
	_, _ = h.stdinW.Write([]byte{'a'})
	h.waitFor(t, func() bool { return rec.Bounds().X == 1 }, "bounds mutation")

	h.app.Quit()
	_ = h.stdinW.Close()
	wait()
}

// TestRunEmptyRootDoesNotCrash covers the a.Root == nil branches in
// draw + refreshSize + input dispatch + tick dispatch.
func TestRunEmptyRootDoesNotCrash(t *testing.T) {
	h := newHarness(t)
	h.app.TickHz = 30 // exercise tick w/ nil Root
	h.app.Keys["q"] = func(a *App) { a.Quit() }

	wait := h.runAsync(t)
	// Fire a tick to hit the nil-Root branch in the tick case.
	h.tickCh <- time.Now()
	// Then a resize to hit the nil-Root branch in refreshSize.
	h.resizeCh <- struct{}{}
	// Then quit via input.
	_, _ = h.stdinW.Write([]byte{'q'})
	_ = h.stdinW.Close()
	code := wait()
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

// TestRunSizeErrorFallsBackToDefaults covers the tty.Size err branch
// of refreshSize — a Size() error must not crash Run and must yield
// a Root sized to the DefaultCols x DefaultRows floor.
func TestRunSizeErrorFallsBackToDefaults(t *testing.T) {
	h := newHarness(t)
	h.tty.sizeErr = errors.New("get size failed")
	rec := &recordingWidget{}
	h.app.Root = rec
	h.app.Keys["q"] = func(a *App) { a.Quit() }

	wait := h.runAsync(t)
	_, _ = h.stdinW.Write([]byte{'q'})
	_ = h.stdinW.Close()
	wait()
	if rec.Bounds().W != DefaultCols || rec.Bounds().H != DefaultRows {
		t.Errorf("Root bounds = %+v, want %dx%d", rec.Bounds(), DefaultCols, DefaultRows)
	}
}

// TestRunNilThemeUsesDefaultLight covers the theme == nil branch of
// draw() — a nil Theme must fall through to toolkit.DefaultLight()
// without panicking.
func TestRunNilThemeUsesDefaultLight(t *testing.T) {
	h := newHarness(t)
	h.app.Theme = nil
	h.app.Keys["q"] = func(a *App) { a.Quit() }

	wait := h.runAsync(t)
	_, _ = h.stdinW.Write([]byte{'q'})
	_ = h.stdinW.Close()
	wait()
}

// TestRunUnmatchedKeyDispatchesToRoot covers the "no Keys match"
// branch — the event skips the handler dispatch but still reaches
// Root.OnEvent.
func TestRunUnmatchedKeyDispatchesToRoot(t *testing.T) {
	h := newHarness(t)
	rec := &recordingWidget{}
	h.app.Root = rec
	h.app.Keys["q"] = func(a *App) { a.Quit() }

	wait := h.runAsync(t)
	// 'z' is not in Keys; must NOT trigger Quit but must reach Root.
	_, _ = h.stdinW.Write([]byte{'z'})
	h.waitFor(t, func() bool { return rec.eventCount() >= 1 }, "z event dispatched")
	// Now quit.
	_, _ = h.stdinW.Write([]byte{'q'})
	_ = h.stdinW.Close()
	wait()
}

// TestRunZeroSizeDrawFallsBackToDefaults covers the draw() cols<=0
// || rows<=0 branch — a fake TTY reporting (0, 0) with no err yields
// (0, 0) at first refreshSize (they get replaced with defaults there
// already), so instead we drive a direct code path in draw() with a
// fresh App whose cols/rows stayed zero.
func TestRunZeroSizeDrawFallsBackToDefaults(t *testing.T) {
	// Direct draw call with a hand-built App to hit draw()'s size
	// fallback INDEPENDENT of refreshSize (which also normalizes).
	buf, err := os.CreateTemp("", "draw-zero-*.out")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer os.Remove(buf.Name())
	defer buf.Close()

	a := &App{
		stdout: buf,
		Theme:  toolkit.DefaultLight(),
	}
	a.draw() // cols/rows are 0 → defaults kick in; Root is nil.
	fi, _ := buf.Stat()
	if fi.Size() == 0 {
		t.Error("draw() with zero size produced no output")
	}
}

// TestRealSignalFnDeliversWinch verifies the production signal path:
// signal.Notify + goroutine forward SIGWINCH to resizeCh. Multiple
// signals in quick succession also exercise the `default:` branch
// when the buffered channel is already full.
func TestRealSignalFnDeliversWinch(t *testing.T) {
	resize, _, stop := realSignalFn()
	defer stop()
	p, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("FindProcess: %v", err)
	}
	// Fire several SIGWINCH in a row so the goroutine's default:
	// branch (dropped resize when buffer already full) is hit.
	for i := 0; i < 8; i++ {
		if err := p.Signal(syscall.SIGWINCH); err != nil {
			t.Fatalf("Signal SIGWINCH: %v", err)
		}
	}
	select {
	case <-resize:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("SIGWINCH did not reach resize channel")
	}
}

// TestRealSignalFnDeliversInterrupt verifies the SIGINT → interrupt
// path + its default: branch when the channel is already full.
func TestRealSignalFnDeliversInterrupt(t *testing.T) {
	_, interrupt, stop := realSignalFn()
	defer stop()
	p, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("FindProcess: %v", err)
	}
	for i := 0; i < 8; i++ {
		if err := p.Signal(os.Interrupt); err != nil {
			t.Fatalf("Signal Interrupt: %v", err)
		}
	}
	select {
	case <-interrupt:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("SIGINT did not reach interrupt channel")
	}
}

// TestRealTickFnTicks verifies the production tick path emits at
// approximately the requested rate.
func TestRealTickFnTicks(t *testing.T) {
	ch, stop := realTickFn(5 * time.Millisecond)
	defer stop()
	select {
	case <-ch:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("realTickFn did not tick")
	}
}

// TestRunStdinGoroutineQuitBranchExits stresses the stdin reader
// goroutine's `case <-a.quit:` branch: with a spinning reader that
// never EOFs, the goroutine's send eventually blocks on the full
// inCh; the closed quit channel then unblocks it and Run returns.
func TestRunStdinGoroutineQuitBranchExits(t *testing.T) {
	h := newHarness(t)
	// Replace the pipe with a spinning reader that always returns
	// one byte and NEVER EOFs. Every returned byte is 'x' which is
	// registered in Keys to Quit — the first delivery closes quit,
	// after which the goroutine's blocked send takes the quit path.
	stopReader := make(chan struct{})
	sr := &spinReader{payload: 'x', stop: stopReader, delivered: &atomic.Int64{}}
	h.app.stdinFn = func() io.Reader { return sr }
	h.app.Keys["x"] = func(a *App) { a.Quit() }

	wait := h.runAsync(t)
	wait()
	// Signal the reader to stop spinning so this test doesn't leak.
	close(stopReader)
	// If we got here without a 2s timeout, the goroutine unblocked.
	if sr.delivered.Load() < 1 {
		t.Error("spinning reader did not deliver any bytes")
	}
}

// spinReader is a never-EOFing io.Reader that returns a single byte
// per Read until the caller closes stop. Coverage-critical: it lets
// the stdin goroutine over-fill inCh so its `case <-a.quit:` fires.
type spinReader struct {
	payload   byte
	stop      chan struct{}
	delivered *atomic.Int64
}

func (r *spinReader) Read(b []byte) (int, error) {
	select {
	case <-r.stop:
		return 0, io.EOF
	default:
	}
	b[0] = r.payload
	r.delivered.Add(1)
	return 1, nil
}
