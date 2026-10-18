package app

import (
	"testing"

	"github.com/tonk/tuios/internal/config"
	"github.com/tonk/tuios/internal/terminal"
)

func TestKeepWindowNumbers(t *testing.T) {
	old := config.KeepWindowNumbers
	t.Cleanup(func() { config.KeepWindowNumbers = old })

	build := func() (*OS, []*terminal.Window) {
		ws := []*terminal.Window{
			{Workspace: 1}, {Workspace: 1}, {Workspace: 1}, {Workspace: 2},
		}
		return &OS{Windows: ws, CurrentWorkspace: 1, NumWorkspaces: 9}, ws
	}
	numbers := func(m *OS, ws []*terminal.Window) []int {
		out := make([]int, len(ws))
		for i, w := range ws {
			out[i] = m.workspacePosition(w)
		}
		return out
	}
	equal := func(a, b []int) bool {
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return len(a) == len(b)
	}

	t.Run("renumbered by default", func(t *testing.T) {
		config.KeepWindowNumbers = false
		m, ws := build()
		m.Windows = append(ws[:1:1], ws[2:]...) // close the middle one
		if got, want := numbers(m, m.Windows), []int{1, 2, 1}; !equal(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("kept when a middle window closes", func(t *testing.T) {
		config.KeepWindowNumbers = true
		m, ws := build()
		if got, want := numbers(m, ws), []int{1, 2, 3, 1}; !equal(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
		m.Windows = append(ws[:1:1], ws[2:]...)
		if got, want := numbers(m, m.Windows), []int{1, 3, 1}; !equal(got, want) {
			t.Fatalf("after close: got %v, want %v", got, want)
		}
		// The next window fills the gap.
		extra := &terminal.Window{Workspace: 1}
		m.Windows = append(m.Windows, extra)
		if got := m.workspacePosition(extra); got != 2 {
			t.Fatalf("new window numbered %d, want 2", got)
		}
	})
}
