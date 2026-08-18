package luascript

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Gaurav-Gosain/tuios/internal/tape"
	lua "github.com/yuin/gopher-lua"
)

// call records one Executor method invocation for assertions.
type call struct {
	method string
	args   []string
}

// fakeExecutor implements tape.Executor by recording calls instead of
// touching real app state. Methods not overridden panic if called (embedded
// nil interface), same pattern as tape.nopExecutor.
type fakeExecutor struct {
	tape.Executor

	mu        sync.Mutex
	calls     []call
	content   string
	focusedID string
}

func (f *fakeExecutor) log(method string, args ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call{method, args})
}

func (f *fakeExecutor) Calls() []call {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]call(nil), f.calls...)
}

func (f *fakeExecutor) SetContent(s string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.content = s
}

func (f *fakeExecutor) CreateNewWindow() error { f.log("CreateNewWindow"); return nil }
func (f *fakeExecutor) CreateNewWindowWithName(name string) error {
	f.log("CreateNewWindowWithName", name)
	return nil
}
func (f *fakeExecutor) SendToWindow(id string, data []byte) error {
	f.log("SendToWindow", id, string(data))
	return nil
}
func (f *fakeExecutor) GetFocusedWindowID() string { return f.focusedID }
func (f *fakeExecutor) GetWindowContent(_ string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.content, nil
}
func (f *fakeExecutor) SwitchWorkspace(ws int) error {
	f.log("SwitchWorkspace", strconv.Itoa(ws))
	return nil
}
func (f *fakeExecutor) ShowNotificationCmd(msg, kind string) error {
	f.log("ShowNotificationCmd", msg, kind)
	return nil
}
func (f *fakeExecutor) SplitHorizontal() error { f.log("SplitHorizontal"); return nil }
func (f *fakeExecutor) SetTheme(name string) error {
	f.log("SetTheme", name)
	return nil
}

// runScript runs a Lua tape script to completion against exec, pumping the
// bridge itself instead of routing through Bubble Tea (bridge.Listen() is a
// thin tea.Cmd wrapper over the same reqCh this drains directly). It fails
// the test if the script doesn't finish within timeout.
func runScript(t *testing.T, script string, exec *fakeExecutor, timeout time.Duration) error {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ce := tape.NewCommandExecutor(exec)
	bridge := NewBridge()

	L := lua.NewState(lua.Options{SkipOpenLibs: true})
	defer L.Close()
	OpenSafeLibs(L)
	L.SetContext(ctx)
	Register(L, ce, exec, bridge, ctx)

	done := make(chan error, 1)
	go func() { done <- L.DoString(script) }()

	for {
		select {
		case fn := <-bridge.reqCh:
			fn()
		case err := <-done:
			return err
		case <-ctx.Done():
			t.Fatalf("script did not finish within %s", timeout)
			return ctx.Err()
		}
	}
}

func TestDispatchVerbsCallThroughToExecutor(t *testing.T) {
	exec := &fakeExecutor{}
	script := `
		tuios.new_window("work")
		tuios.type("echo hi")
		tuios.split("horizontal")
		tuios.switch_workspace(2)
		tuios.notify("hello", "info")
		tuios.set_theme("dracula")
	`
	if err := runScript(t, script, exec, 2*time.Second); err != nil {
		t.Fatalf("script failed: %v", err)
	}

	want := []call{
		{"CreateNewWindowWithName", []string{"work"}},
		{"SendToWindow", []string{"", "echo hi"}},
		{"SplitHorizontal", nil},
		{"SwitchWorkspace", []string{"2"}},
		{"ShowNotificationCmd", []string{"hello", "info"}},
		{"SetTheme", []string{"dracula"}},
	}
	got := exec.Calls()
	if len(got) != len(want) {
		t.Fatalf("got %d calls, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].method != w.method || strings.Join(got[i].args, ",") != strings.Join(w.args, ",") {
			t.Errorf("call %d = %+v, want %+v", i, got[i], w)
		}
	}
}

func TestSleepBlocksForRoughlyTheRequestedDuration(t *testing.T) {
	exec := &fakeExecutor{}
	start := time.Now()
	if err := runScript(t, `tuios.sleep(150)`, exec, 2*time.Second); err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 150*time.Millisecond {
		t.Errorf("sleep(150) returned after %s, want >= 150ms", elapsed)
	}
}

func TestWaitUntilReturnsTrueOnMatch(t *testing.T) {
	exec := &fakeExecutor{}
	exec.SetContent("$ ")

	go func() {
		time.Sleep(60 * time.Millisecond)
		exec.SetContent("Password: ")
	}()

	script := `
		local matched = tuios.wait_until("[Pp]assword:", 2000)
		if not matched then error("expected a match") end
	`
	if err := runScript(t, script, exec, 3*time.Second); err != nil {
		t.Fatalf("script failed: %v", err)
	}
}

func TestWaitUntilReturnsFalseOnTimeoutInsteadOfErroring(t *testing.T) {
	exec := &fakeExecutor{}
	exec.SetContent("$ ")

	script := `
		local matched = tuios.wait_until("nope", 100)
		if matched then error("expected no match") end
	`
	if err := runScript(t, script, exec, 2*time.Second); err != nil {
		t.Fatalf("script failed: %v", err)
	}
}

func TestContextCancellationStopsAScript(t *testing.T) {
	exec := &fakeExecutor{}
	ctx, cancel := context.WithCancel(context.Background())

	ce := tape.NewCommandExecutor(exec)
	bridge := NewBridge()
	L := lua.NewState(lua.Options{SkipOpenLibs: true})
	defer L.Close()
	OpenSafeLibs(L)
	L.SetContext(ctx)
	Register(L, ce, exec, bridge, ctx)

	done := make(chan error, 1)
	go func() { done <- L.DoString(`tuios.wait_until("never", 60000)`) }()

	time.Sleep(30 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Error("expected an error from a canceled script, got nil")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("canceling the context did not stop the script")
	}
}

func TestSandboxHasNoFilesystemOrProcessAccess(t *testing.T) {
	for _, script := range []string{
		`if os ~= nil then error("os should not be open") end`,
		`if io ~= nil then error("io should not be open") end`,
		`if dofile ~= nil then error("dofile should be nil") end`,
		`if require ~= nil then error("require should be nil") end`,
		`if loadfile ~= nil then error("loadfile should be nil") end`,
	} {
		exec := &fakeExecutor{}
		if err := runScript(t, script, exec, time.Second); err != nil {
			t.Errorf("sandbox check failed for %q: %v", script, err)
		}
	}
}
