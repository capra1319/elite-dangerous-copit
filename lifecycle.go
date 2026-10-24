package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
)

// Bubble Tea does not join command goroutines. Own our timers, journal reads,
// process probes and backend so they stop before a session can be replaced.
type sessionWork struct {
	ctx      context.Context
	cancel   context.CancelFunc
	mu       sync.Mutex
	stopping bool
	wg       sync.WaitGroup
}

func newSessionWork(ctx context.Context) *sessionWork {
	child, cancel := context.WithCancel(ctx)
	return &sessionWork{ctx: child, cancel: cancel}
}

func (w *sessionWork) command(delay time.Duration, work tea.Cmd) tea.Cmd {
	return func() tea.Msg {
		if w == nil {
			if delay > 0 {
				time.Sleep(delay)
			}
			return work()
		}
		w.mu.Lock()
		if w.stopping || w.ctx.Err() != nil {
			w.mu.Unlock()
			return nil
		}
		w.wg.Add(1)
		w.mu.Unlock()
		defer w.wg.Done()
		if delay > 0 {
			timer := time.NewTimer(delay)
			defer timer.Stop()
			select {
			case <-w.ctx.Done():
				return nil
			case <-timer.C:
			}
		}
		if w.ctx.Err() != nil {
			return nil
		}
		return work()
	}
}

func (w *sessionWork) stop() <-chan struct{} {
	w.mu.Lock()
	w.stopping = true
	w.cancel()
	w.mu.Unlock()
	done := make(chan struct{})
	go func() { w.wg.Wait(); close(done) }()
	return done
}

func safeBackend(backend tea.Cmd) (msg tea.Msg) {
	defer func() {
		if failure := recover(); failure != nil {
			msg = backendDoneMsg{err: fmt.Errorf("game backend stopped after panic: %v", failure)}
		}
	}()
	return backend()
}
