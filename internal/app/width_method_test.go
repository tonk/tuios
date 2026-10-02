package app

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// widthProbeModel draws one line holding a VS16 emoji after switching the
// program to grapheme width the way OS.Init does, then quits.
type widthProbeModel struct{ switched bool }

func (w *widthProbeModel) Init() tea.Cmd { return reportGraphemeWidth }

func (w *widthProbeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if report, ok := msg.(tea.ModeReportMsg); ok && report.Mode == ansi.ModeUnicodeCore {
		w.switched = true
		return w, tea.Quit
	}
	return w, nil
}

func (w *widthProbeModel) View() tea.View { return tea.NewView("❤️x|") }

// TestReportGraphemeWidth_SwitchesBubbleTea runs a real program and checks that
// the report reportGraphemeWidth delivers reaches Bubble Tea's event loop, which
// is the only supported way to put its cell buffer on grapheme width: the
// renderer answers by writing DECSET ?2027 to the host. If a Bubble Tea
// upgrade stopped honouring a synthesised report, TUIOS's frames would be
// re-measured with wcwidth again and drift a column after every VS16 emoji.
func TestReportGraphemeWidth_SwitchesBubbleTea(t *testing.T) {
	msg := reportGraphemeWidth()
	report, ok := msg.(tea.ModeReportMsg)
	if !ok || report.Mode != ansi.ModeUnicodeCore || !report.Value.IsSet() {
		t.Fatalf("reportGraphemeWidth() = %#v, want a set ModeUnicodeCore report", msg)
	}

	var out bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	model := &widthProbeModel{}
	p := tea.NewProgram(model,
		tea.WithContext(ctx),
		tea.WithInput(nil),
		tea.WithOutput(&out),
		tea.WithWindowSize(20, 3),
		tea.WithoutSignals(),
		tea.WithEnvironment([]string{"TERM=dumb"}),
	)
	if _, err := p.Run(); err != nil {
		t.Fatalf("program run: %v", err)
	}
	if !model.switched {
		t.Fatal("the mode report never reached the model")
	}
	if !strings.Contains(out.String(), ansi.SetModeUnicodeCore) {
		t.Fatalf("Bubble Tea did not switch to grapheme width (no DECSET ?2027 in output): %q", out.String())
	}
}

// TestOSInitSwitchesToGraphemeWidth checks the wiring: OS.Init has to
// deliver the report, or the program stays on wcwidth.
func TestOSInitSwitchesToGraphemeWidth(t *testing.T) {
	m := &OS{WorkspaceFocus: map[int]int{}, NumWorkspaces: 9}
	want := reflect.ValueOf(tea.Cmd(reportGraphemeWidth)).Pointer()
	for _, cmd := range m.initCmds() {
		if cmd != nil && reflect.ValueOf(cmd).Pointer() == want {
			return
		}
	}
	t.Fatal("OS.Init does not schedule reportGraphemeWidth; Bubble Tea would measure frames with wcwidth")
}
