package input

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/tonk/tuios/internal/app"
)

// selectionMenuOffersOpen drags a selection from x0 to x1 on a pane showing
// line, right-clicks it, and reports whether the selection menu offers
// "Open in browser".
func selectionMenuOffersOpen(t *testing.T, o *app.OS, x0, x1 int) bool {
	t.Helper()
	pressAt(o, x0, 0)
	dragTo(o, x1, 0)
	release(o, x1, 0)

	handleMouseClick(tea.MouseClickMsg{X: x0 + 1, Y: 1, Button: tea.MouseRight}, o)
	if !o.ContextMenuActive() {
		t.Fatal("right-clicking a selection opened no menu")
	}
	for _, it := range o.ContextMenu.Items {
		if it.Action == "open_selection_url" {
			if it.Dim {
				t.Error("the open row is offered dimmed")
			}
			return true
		}
	}
	return false
}

func TestSelectionMenuOpenInBrowser(t *testing.T) {
	const line = "see https://example.com/a. and more"
	tests := []struct {
		name   string
		x0, x1 int
		ssh    bool
		want   bool
	}{
		{"exactly the URL", 4, 24, false, true},
		{"URL with its full stop", 4, 25, false, true},
		{"URL with blanks around it", 3, 26, false, true},
		{"a plain word", 27, 29, false, false},
		{"a line that mentions a URL", 0, 34, false, false},
		{"part of the URL", 4, 10, false, false},
		{"over SSH", 4, 24, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stubURLOpener(t)
			o, _ := selectPane(t, line)
			o.IsSSHMode = tt.ssh
			if got := selectionMenuOffersOpen(t, o, tt.x0, tt.x1); got != tt.want {
				t.Errorf("menu offers Open in browser = %v, want %v", got, tt.want)
			}
		})
	}
}

// The row runs the action, which opens the selected URL.
func TestOpenSelectionURLOpensTheSelection(t *testing.T) {
	opened := stubURLOpener(t)
	o, _ := selectPane(t, "see https://example.com/a. and more")
	if !selectionMenuOffersOpen(t, o, 4, 25) {
		t.Fatal("no Open in browser row")
	}
	o.CloseContextMenu()
	runContextMenuAction("open_selection_url", o)
	if len(*opened) != 1 || (*opened)[0] != "https://example.com/a" {
		t.Errorf("opened %v, want [https://example.com/a]", *opened)
	}
}

// The row does not depend on open_url_on_double_click: that option is about
// the double-click, and the row is an explicit request.
func TestSelectionMenuOpenIgnoresTheDoubleClickOption(t *testing.T) {
	stubURLOpener(t)
	o, _ := selectPane(t, "https://example.com")
	disableDoubleClickOpen(t)
	if !selectionMenuOffersOpen(t, o, 0, 18) {
		t.Error("turning off the double-click option hid the menu row")
	}
}
