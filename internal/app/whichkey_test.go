package app

import (
	"strings"
	"testing"

	"github.com/tonk/tuios/internal/config"
)

func TestBuildWhichKeyColumnsUsesMultipleColumns(t *testing.T) {
	groups := config.GetPrefixKeybindingGroups("", nil, false)
	cols := buildWhichKeyColumns(groups, 120)
	if len(cols) < 2 {
		t.Fatalf("wide screen should pack leader which-key into multiple columns, got %d", len(cols))
	}

	joined := renderWhichKeyPanel("Prefix", groups, 120, 40)
	for _, title := range []string{"windows", "layout", "navigate", "tools", "session"} {
		if !strings.Contains(joined, title) {
			t.Errorf("rendered which-key missing group label %q\n%s", title, joined)
		}
	}
}

func TestBuildWhichKeyColumnsFallsBackOnNarrowScreens(t *testing.T) {
	groups := config.GetPrefixKeybindingGroups("", nil, false)
	cols := buildWhichKeyColumns(groups, 40)
	if len(cols) != 1 {
		t.Fatalf("narrow screen should keep a single column, got %d", len(cols))
	}
}

func TestWhichKeyPanelFitsHeight(t *testing.T) {
	groups := config.GetPrefixKeybindingGroups("", nil, false)
	// Short screen: multi-column plus truncation must still produce a panel
	// no taller than the screen.
	const h = 20
	rendered := renderWhichKeyPanel("Prefix", groups, 100, h)
	if got := strings.Count(rendered, "\n") + 1; got > h {
		t.Fatalf("which-key panel height %d exceeds screen %d", got, h)
	}
}
