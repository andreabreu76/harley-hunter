package browser

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestFlagsCarryTheProfileAndTheHiddenWindow(t *testing.T) {
	got := Flags(Options{ProfileDir: "/tmp/profile"}, 9333)
	joined := strings.Join(got, " ")

	for _, want := range []string{
		"--remote-debugging-port=9333",
		"--user-data-dir=/tmp/profile",
		"--no-first-run",
		"--no-default-browser-check",
		"--window-position=-32000,-32000",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("flags miss %q:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "--headless") {
		t.Errorf("a windowed launch asked for headless:\n%s", joined)
	}
}

func TestFlagsGoHeadlessWithoutAScreen(t *testing.T) {
	joined := strings.Join(Flags(Options{ProfileDir: "/tmp/profile", Headless: true}, 9333), " ")
	if !strings.Contains(joined, "--headless=new") {
		t.Errorf("headless launch missed the flag:\n%s", joined)
	}
	if strings.Contains(joined, "--window-position") {
		t.Errorf("headless launch still asked for a window position:\n%s", joined)
	}
}

func TestHeadlessNeededOnlyOnLinuxWithoutADisplay(t *testing.T) {
	empty := func(string) string { return "" }
	withX11 := func(key string) string {
		if key == "DISPLAY" {
			return ":0"
		}
		return ""
	}
	withWayland := func(key string) string {
		if key == "WAYLAND_DISPLAY" {
			return "wayland-0"
		}
		return ""
	}

	if !HeadlessNeeded("linux", empty) {
		t.Error("a Linux server without a display was not sent to headless")
	}
	if HeadlessNeeded("linux", withX11) {
		t.Error("a Linux desktop on X11 was sent to headless")
	}
	if HeadlessNeeded("linux", withWayland) {
		t.Error("a Linux desktop on Wayland was sent to headless")
	}
	if HeadlessNeeded("darwin", empty) || HeadlessNeeded("windows", empty) {
		t.Error("macOS or Windows was sent to headless")
	}
}

func TestLaunchReusesAnExistingBrowserAndNeverKillsIt(t *testing.T) {
	started := 0
	killed := 0
	d := deps{
		start:    func(*exec.Cmd) error { started++; return nil },
		wait:     func(*exec.Cmd) error { return nil },
		kill:     func(*exec.Cmd) error { killed++; return nil },
		probe:    func(string) error { return nil },
		freePort: func() (int, error) { return 9333, nil },
		sleep:    func(time.Duration) {},
	}

	h, err := launch(context.Background(), Options{ExistingURL: "http://127.0.0.1:9222"}, d)
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	if started != 0 {
		t.Errorf("started %d browsers while one was already answering", started)
	}
	if h.Owned() {
		t.Error("the handle claims a browser it did not start")
	}
	if err := h.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if killed != 0 {
		t.Errorf("Close killed a browser it did not start")
	}
}

func TestLaunchFailsWhenTheConfiguredBrowserIsNotAnswering(t *testing.T) {
	d := deps{
		start:    func(*exec.Cmd) error { t.Fatal("started a browser instead of failing"); return nil },
		wait:     func(*exec.Cmd) error { return nil },
		kill:     func(*exec.Cmd) error { return nil },
		probe:    func(string) error { return errors.New("connection refused") },
		freePort: func() (int, error) { return 9333, nil },
		sleep:    func(time.Duration) {},
	}

	_, err := launch(context.Background(), Options{ExistingURL: "http://127.0.0.1:9222"}, d)
	if err == nil {
		t.Fatal("launch accepted a devtools_url that answers nothing")
	}
	if !strings.Contains(err.Error(), "devtools_url") {
		t.Errorf("error = %q, want it to name the setting that has to change", err)
	}
}

func TestLaunchStartsAndClosesItsOwnBrowser(t *testing.T) {
	started := 0
	killed := 0
	answers := false
	d := deps{
		start: func(*exec.Cmd) error { started++; answers = true; return nil },
		wait:  func(*exec.Cmd) error { return nil },
		kill:  func(*exec.Cmd) error { killed++; return nil },
		probe: func(string) error {
			if answers {
				return nil
			}
			return errors.New("connection refused")
		},
		freePort: func() (int, error) { return 9444, nil },
		sleep:    func(time.Duration) {},
	}

	h, err := launch(context.Background(), Options{ExecutablePath: "/usr/bin/chromium", ProfileDir: "/tmp/profile"}, d)
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	if started != 1 {
		t.Errorf("started %d browsers, want 1", started)
	}
	if got, want := h.DevtoolsURL(), "http://127.0.0.1:9444"; got != want {
		t.Errorf("DevtoolsURL = %q, want %q", got, want)
	}
	if !h.Owned() {
		t.Error("the handle does not claim the browser it started")
	}
	if err := h.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if killed != 1 {
		t.Errorf("Close killed %d browsers, want 1", killed)
	}
}

func TestLaunchKillsTheBrowserThatNeverAnswers(t *testing.T) {
	killed := 0
	d := deps{
		start:    func(*exec.Cmd) error { return nil },
		wait:     func(*exec.Cmd) error { return nil },
		kill:     func(*exec.Cmd) error { killed++; return nil },
		probe:    func(string) error { return errors.New("connection refused") },
		freePort: func() (int, error) { return 9444, nil },
		sleep:    func(time.Duration) {},
	}

	if _, err := launch(context.Background(), Options{ExecutablePath: "/usr/bin/chromium", ProfileDir: "/tmp/profile"}, d); err == nil {
		t.Fatal("launch returned a handle for a browser that never answered")
	}
	if killed != 1 {
		t.Errorf("a browser that never answered was left running (killed = %d)", killed)
	}
}

func TestLaunchReapsTheBrowserItStarted(t *testing.T) {
	waited := make(chan struct{}, 1)
	d := deps{
		start:    func(*exec.Cmd) error { return nil },
		wait:     func(*exec.Cmd) error { waited <- struct{}{}; return nil },
		kill:     func(*exec.Cmd) error { return nil },
		probe:    func(string) error { return nil },
		freePort: func() (int, error) { return 9444, nil },
		sleep:    func(time.Duration) {},
	}

	h, err := launch(context.Background(), Options{ExecutablePath: "/usr/bin/chromium", ProfileDir: "/tmp/profile"}, d)
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	defer h.Close()

	select {
	case <-waited:
	case <-time.After(time.Second):
		t.Fatal("nobody is waiting on the browser: killing it leaves a zombie and the watchdog goroutine behind")
	}
}

func TestCloseAcceptsABrowserThatHadAlreadyExited(t *testing.T) {
	d := deps{
		start:    func(*exec.Cmd) error { return nil },
		wait:     func(*exec.Cmd) error { return nil },
		kill:     func(*exec.Cmd) error { return os.ErrProcessDone },
		probe:    func(string) error { return nil },
		freePort: func() (int, error) { return 9444, nil },
		sleep:    func(time.Duration) {},
	}

	h, err := launch(context.Background(), Options{ExecutablePath: "/usr/bin/chromium", ProfileDir: "/tmp/profile"}, d)
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	if err := h.Close(); err != nil {
		t.Errorf("Close complained about a browser that had already exited on its own: %v", err)
	}
}
