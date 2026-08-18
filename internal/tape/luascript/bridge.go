// Package luascript runs .lua tape scripts against the same tape.Executor
// used by the .tape DSL, giving power users real variables, loops and
// conditionals without touching the DSL's lexer/parser/Player.
package luascript

import (
	"context"

	tea "charm.land/bubbletea/v2"
)

// Bridge lets a Lua script's goroutine run a closure on the Bubble Tea
// Update() goroutine and block until it completes. This mirrors
// internal/app's ListenForPTYData: a channel-blocking closure handed to
// Bubble Tea as a tea.Cmd, which Bubble Tea runs on its own goroutine and
// turns into a message back into Update() on wakeup.
//
// This exists because Update() is the only goroutine allowed to touch
// window/executor state, but a Lua script must run off that goroutine (a
// tight loop or a sleep in the script must never freeze the UI).
type Bridge struct {
	reqCh chan func()
}

// NewBridge creates a Bridge ready to relay calls into Update().
func NewBridge() *Bridge {
	return &Bridge{reqCh: make(chan func())}
}

// Call runs fn on the Update() goroutine and blocks the calling (Lua)
// goroutine until it finishes, or ctx is canceled first.
func (b *Bridge) Call(ctx context.Context, fn func()) error {
	done := make(chan struct{})
	wrapped := func() {
		fn()
		close(done)
	}

	select {
	case b.reqCh <- wrapped:
	case <-ctx.Done():
		return ctx.Err()
	}

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// CallMsg carries a pending Lua-triggered closure into Update(). The handler
// must run Fn synchronously and then re-arm Listen if the script is still
// running.
type CallMsg struct{ Fn func() }

// Listen returns a tea.Cmd that waits for the next queued Call and delivers
// it as a CallMsg. Update() must call this again after handling a CallMsg to
// keep relaying subsequent calls.
func (b *Bridge) Listen() tea.Cmd {
	return func() tea.Msg {
		fn := <-b.reqCh
		return CallMsg{Fn: fn}
	}
}
