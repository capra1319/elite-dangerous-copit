package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestSessionWorkCancelsTimersJoinsActiveWorkAndRejectsLateCommands(t *testing.T) {
	w := newSessionWork(context.Background())
	started, release := make(chan struct{}), make(chan struct{})
	timerStarted, timerDone := make(chan struct{}), make(chan struct{})
	go func() {
		close(timerStarted)
		w.command(time.Hour, func() tea.Msg { t.Error("canceled timer ran"); return nil })()
		close(timerDone)
	}()
	<-timerStarted
	go w.command(0, func() tea.Msg { close(started); <-release; return nil })()
	<-started
	stopped := w.stop()
	select {
	case <-stopped:
		t.Fatal("shutdown returned before active work finished")
	default:
	}
	select {
	case <-timerDone:
	case <-time.After(time.Second):
		t.Fatal("timer ignored cancellation")
	}
	w.command(0, func() tea.Msg { t.Error("late command ran after shutdown"); return nil })()
	close(release)
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("finished worker was not joined")
	}
}

func TestCockpitSessionsJoinWorkersOnQuit(t *testing.T) {
	for _, mode := range []string{"mock", "live"} {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		dir := t.TempDir()
		result, err := runCockpitSession(ctx, cockpitOptions{mode: mode, path: filepath.Join(dir, "Status.json"), journal: dir}, strings.NewReader("q"), io.Discard)
		cancel()
		if err != nil || result.work == nil || result.work.ctx.Err() == nil {
			t.Fatal("session did not cancel its workers on Quit", mode, err)
		}
		select {
		case <-result.work.stop():
		case <-time.After(time.Second):
			t.Fatal("session returned with active workers", mode)
		}
	}
}

func TestBackendPanicUnwindsCleanupAndBecomesAnError(t *testing.T) {
	cleaned := false
	msg := safeBackend(func() tea.Msg { defer func() { cleaned = true }(); panic("injected backend failure") })
	done, ok := msg.(backendDoneMsg)
	if !ok || !cleaned || done.err == nil || !strings.Contains(done.err.Error(), "injected backend failure") {
		t.Fatal("panic lost cleanup or error", msg)
	}
	m := newCockpit(cockpitOptions{mode: "live"}, testNow)
	m.work = newSessionWork(context.Background())
	m = updateCockpit(t, m, done)
	if m.work.ctx.Err() == nil || m.fatal == nil || m.frame.ControlStatus != "DISARMED" {
		t.Fatal("backend failure left companion work running")
	}
	<-m.work.stop()
	w := newSessionWork(context.Background())
	safeBackend(w.command(0, func() tea.Msg { panic("worker failure") }))
	select {
	case <-w.stop():
	case <-time.After(time.Second):
		t.Fatal("panic leaked worker accounting")
	}
}

func TestMockTextCancellationAndQuitJoinTheInputReader(t *testing.T) {
	for _, quit := range []bool{false, true} {
		reader, writer := io.Pipe()
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan int, 1)
		go func() { done <- runMock(ctx, reader, display{out: io.Discard}, false) }()
		if quit {
			if _, err := writer.Write([]byte("quit\n")); err != nil {
				t.Fatal(err)
			}
		} else {
			cancel()
		}
		select {
		case code := <-done:
			if code != 0 {
				t.Fatal("mock shutdown failed")
			}
		case <-time.After(time.Second):
			cancel()
			writer.Close()
			t.Fatal("mock input did not stop")
		}
		cancel()
		if _, err := writer.Write([]byte("b1\n")); !errors.Is(err, io.ErrClosedPipe) {
			t.Fatal("mock reader survived return", err)
		}
		writer.Close()
	}
}

func TestCanceledReadsAndActionsDoNoWork(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := readWithRetry(ctx, "does-not-exist.json"); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled read accessed the path", err)
	}
	engine, sink := testEngine(t)
	engine.Armed = true
	if err := engine.trigger(ctx, "t3", testNow, testTarget(), nil); !errors.Is(err, context.Canceled) || engine.Armed || len(sink.keys) != 0 {
		t.Fatal("canceled action was not stopped and disarmed", err)
	}
}

func TestQueuedHotkeysCannotRearmOrCrossAnArmingSession(t *testing.T) {
	for _, test := range []struct {
		id                 int
		posted, now, armed uint32
		want               bool
	}{
		{1, 100, 401, 0, false}, {1, 100, 400, 0, true}, {2, 1, 9000, 0, true},
		{100, 990, 1100, 1000, false}, {100, 1001, 1100, 1000, true},
		{100, 1001, 1400, 1000, false}, {3, 100, 1000, 0, false}, {4, 100, 101, 0, true},
		{999, 100, 101, 0, false}, {100, ^uint32(0) - 20, 15, ^uint32(0) - 30, true},
		{100, ^uint32(0) - 20, 15, 5, false},
	} {
		if got := acceptControlHotkey(test.id, test.posted, test.now, test.armed); got != test.want {
			t.Fatal("unsafe hotkey queue handling", test, got)
		}
	}
}

type releaseFailureAPI struct{ fakePulseAPI }

func (f *releaseFailureAPI) Scan(_ uint16, up bool) error {
	f.scans = append(f.scans, up)
	if up {
		return errors.New("release failed")
	}
	return nil
}
func TestPulseReleaseFailureIsPreservedAlongsideCancellation(t *testing.T) {
	api := &releaseFailureAPI{fakePulseAPI{target: testTarget(), waitErr: context.Canceled}}
	err := deliverPulse(context.Background(), "F1", testTarget(), api)
	if !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "release failed") || len(api.scans) != 2 {
		t.Fatal("key-release failure was hidden", err)
	}
}

func TestJournalRecordAllocationIsBoundedAndCancellationIsChecked(t *testing.T) {
	input := bufio.NewReader(strings.NewReader(strings.Repeat("x", (4<<20)+100) + "\n{}\n"))
	line, consumed, oversized, err := readJournalRecord(context.Background(), input)
	if err != nil || !oversized || len(line) != 0 || consumed != (4<<20)+101 {
		t.Fatal("oversized record was retained", len(line), consumed, err)
	}
	line, _, oversized, err = readJournalRecord(context.Background(), input)
	if err != nil || oversized || !bytes.Equal(line, []byte("{}\n")) {
		t.Fatal("oversized record damaged the next record")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, consumed, _, err = readJournalRecord(ctx, input)
	if !errors.Is(err, context.Canceled) || consumed != 0 {
		t.Fatal("canceled reader consumed input")
	}
}
