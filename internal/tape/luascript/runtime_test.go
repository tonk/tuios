package luascript

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tonk/tuios/internal/tape"
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

	mu             sync.Mutex
	calls          []call
	content        string
	scrollback     string
	processExited  bool
	focusedID      string
	windowData     map[string]any
	windowListData map[string]any
	sessionData    map[string]any
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
func (f *fakeExecutor) GetWindowScrollback(_ string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.scrollback, nil
}
func (f *fakeExecutor) WindowProcessExited(_ string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.processExited, nil
}
func (f *fakeExecutor) GetWindowData(identifier string) (map[string]any, error) {
	f.log("GetWindowData", identifier)
	return f.windowData, nil
}
func (f *fakeExecutor) GetFocusedWindowData() (map[string]any, error) {
	f.log("GetFocusedWindowData")
	return f.windowData, nil
}
func (f *fakeExecutor) GetWindowListData() map[string]any {
	f.log("GetWindowListData")
	return f.windowListData
}
func (f *fakeExecutor) GetSessionInfoData() map[string]any {
	f.log("GetSessionInfoData")
	return f.sessionData
}
func (f *fakeExecutor) SetSessionName(name string) error {
	f.log("SetSessionName", name)
	return nil
}
func (f *fakeExecutor) SetSessionAccent(accent string) error {
	f.log("SetSessionAccent", accent)
	return nil
}
func (f *fakeExecutor) SetAgentState(state, message, source, harness string) error {
	f.log("SetAgentState", state, message, source, harness)
	return nil
}
func (f *fakeExecutor) SwitchWorkspace(ws int) error {
	f.log("SwitchWorkspace", strconv.Itoa(ws))
	return nil
}
func (f *fakeExecutor) SetWorkspaceName(ws int, name string) error {
	f.log("SetWorkspaceName", strconv.Itoa(ws), name)
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
	return runScriptInDir(t, script, exec, timeout, "")
}

// runScriptInDir is runScript with control over the project_dir() value; most
// tests don't care and go through runScript's "" default.
func runScriptInDir(t *testing.T, script string, exec *fakeExecutor, timeout time.Duration, dir string) error {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ce := tape.NewCommandExecutor(exec)
	bridge := NewBridge()

	L := lua.NewState(lua.Options{SkipOpenLibs: true})
	defer L.Close()
	OpenSafeLibs(L)
	L.SetContext(ctx)
	Register(L, ce, exec, bridge, ctx, dir)

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
		tuios.set_workspace_name(2, "IRVN")
		tuios.set_session_name("irvn")
		tuios.set_session_accent("#ff0000")
		tuios.set_agent_state("working", "installing", "report", "claude-code")
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
		{"SetWorkspaceName", []string{"2", "IRVN"}},
		{"SetSessionName", []string{"irvn"}},
		{"SetSessionAccent", []string{"#ff0000"}},
		{"SetAgentState", []string{"working", "installing", "report", "claude-code"}},
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

func TestProjectDirIsExposedAsHostState(t *testing.T) {
	exec := &fakeExecutor{}
	script := `tuios.notify(tuios.project_dir())`
	if err := runScriptInDir(t, script, exec, 2*time.Second, "/some/project/dir"); err != nil {
		t.Fatalf("script failed: %v", err)
	}
	calls := exec.Calls()
	if len(calls) != 1 || calls[0].method != "ShowNotificationCmd" || len(calls[0].args) == 0 || calls[0].args[0] != "/some/project/dir" {
		t.Fatalf("calls = %+v, want a single notify with the project dir", calls)
	}
}

func TestProjectDirDefaultsToEmpty(t *testing.T) {
	exec := &fakeExecutor{}
	script := `if tuios.project_dir() ~= "" then error("expected empty project_dir") end`
	if err := runScript(t, script, exec, 2*time.Second); err != nil {
		t.Fatalf("script failed: %v", err)
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
	Register(L, ce, exec, bridge, ctx, "")

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

func TestWaitUntilMatchesScrollbackWhenRequested(t *testing.T) {
	exec := &fakeExecutor{content: "visible only", scrollback: "scrolled off\nvisible only"}

	script := `
		if tuios.wait_until("scrolled off", 200) then
			tuios.notify("matched-visible", "info")
		end
		if tuios.wait_until("scrolled off", 200, "", true) then
			tuios.notify("matched-scrollback", "info")
		end
	`
	if err := runScript(t, script, exec, 2*time.Second); err != nil {
		t.Fatalf("script failed: %v", err)
	}

	calls := exec.Calls()
	if len(calls) != 1 || calls[0].method != "ShowNotificationCmd" || calls[0].args[0] != "matched-scrollback" {
		t.Fatalf("calls = %+v, want exactly one notify from the scrollback-matching wait_until", calls)
	}
}

func TestWaitForIdleReturnsTrueOnceContentStopsChanging(t *testing.T) {
	exec := &fakeExecutor{content: "building..."}

	go func() {
		time.Sleep(60 * time.Millisecond)
		exec.SetContent("build done")
	}()

	start := time.Now()
	var result bool
	done := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ce := tape.NewCommandExecutor(exec)
	bridge := NewBridge()
	L := lua.NewState(lua.Options{SkipOpenLibs: true})
	defer L.Close()
	OpenSafeLibs(L)
	L.SetContext(ctx)
	Register(L, ce, exec, bridge, ctx, "")
	L.SetGlobal("record", L.NewFunction(func(L *lua.LState) int {
		result = L.ToBool(1)
		return 0
	}))

	go func() { done <- L.DoString(`record(tuios.wait_for_idle(80, 2000))`) }()
	for {
		select {
		case fn := <-bridge.reqCh:
			fn()
		case err := <-done:
			if err != nil {
				t.Fatalf("script failed: %v", err)
			}
			if !result {
				t.Error("wait_for_idle returned false, want true once content stopped changing")
			}
			if elapsed := time.Since(start); elapsed < 140*time.Millisecond {
				t.Errorf("wait_for_idle returned after %s, want >= idle(80ms) after the last change at ~60ms", elapsed)
			}
			return
		case <-ctx.Done():
			t.Fatal("script did not finish in time")
			return
		}
	}
}

func TestWaitForExitReturnsTrueWhenProcessExited(t *testing.T) {
	exec := &fakeExecutor{processExited: true}
	script := `
		if tuios.wait_for_exit(1000) then
			tuios.notify("exited", "info")
		end
	`
	if err := runScript(t, script, exec, 2*time.Second); err != nil {
		t.Fatalf("script failed: %v", err)
	}
	calls := exec.Calls()
	if len(calls) != 1 || calls[0].args[0] != "exited" {
		t.Fatalf("calls = %+v, want a single notify confirming the exit was observed", calls)
	}
}

func TestWaitForExitTimesOutWhenProcessStillRunning(t *testing.T) {
	exec := &fakeExecutor{processExited: false}
	start := time.Now()
	script := `
		if not tuios.wait_for_exit(100) then
			tuios.notify("still-running", "info")
		end
	`
	if err := runScript(t, script, exec, 2*time.Second); err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 100*time.Millisecond {
		t.Errorf("wait_for_exit(100) returned after %s, want >= 100ms", elapsed)
	}
	calls := exec.Calls()
	if len(calls) != 1 || calls[0].args[0] != "still-running" {
		t.Fatalf("calls = %+v, want a single notify confirming the timeout", calls)
	}
}

func TestStructuredQueriesConvertToIndexableLuaTables(t *testing.T) {
	exec := &fakeExecutor{
		windowData: map[string]any{
			"id": "abc123", "workspace": 2, "focused": true,
		},
		windowListData: map[string]any{
			"total": 2,
			"windows": []map[string]any{
				{"id": "abc123", "workspace": 1},
				{"id": "def456", "workspace": 2},
			},
		},
		sessionData: map[string]any{
			"current_workspace": 1, "tiling_enabled": false,
		},
	}

	script := `
		local w = tuios.get_window("abc123")
		tuios.notify(w.id .. "," .. tostring(w.workspace) .. "," .. tostring(w.focused), "info")

		local list = tuios.list_windows()
		tuios.notify(tostring(list.total) .. "," .. list.windows[1].id .. "," .. tostring(list.windows[2].workspace), "info")

		local info = tuios.session_info()
		tuios.notify(tostring(info.current_workspace) .. "," .. tostring(info.tiling_enabled), "info")
	`
	if err := runScript(t, script, exec, 2*time.Second); err != nil {
		t.Fatalf("script failed: %v", err)
	}

	calls := exec.Calls()
	notifies := make([]string, 0, len(calls))
	for _, c := range calls {
		if c.method == "ShowNotificationCmd" {
			notifies = append(notifies, c.args[0])
		}
	}
	want := []string{"abc123,2,true", "2,abc123,2", "1,false"}
	if len(notifies) != len(want) {
		t.Fatalf("notifies = %v, want %v", notifies, want)
	}
	for i, w := range want {
		if notifies[i] != w {
			t.Errorf("notify %d = %q, want %q", i, notifies[i], w)
		}
	}
}
