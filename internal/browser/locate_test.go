package browser

import (
	"errors"
	"strings"
	"testing"
)

func envOf(pairs map[string]string) func(string) string {
	return func(key string) string { return pairs[key] }
}

func existsOnly(paths ...string) func(string) bool {
	present := make(map[string]bool, len(paths))
	for _, p := range paths {
		present[p] = true
	}
	return func(path string) bool { return present[path] }
}

func noLookPath(string) (string, error) { return "", errors.New("not in PATH") }

func TestLocateFindsChromeOnMacOS(t *testing.T) {
	chrome := "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
	got, err := locate("darwin", envOf(nil), noLookPath, existsOnly(chrome))
	if err != nil {
		t.Fatalf("locate: %v", err)
	}
	if got != chrome {
		t.Errorf("locate = %q, want %q", got, chrome)
	}
}

func TestLocatePrefersChromeOverEdge(t *testing.T) {
	chrome := "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
	edge := "/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge"
	got, err := locate("darwin", envOf(nil), noLookPath, existsOnly(edge, chrome))
	if err != nil {
		t.Fatalf("locate: %v", err)
	}
	if got != chrome {
		t.Errorf("locate = %q, want Chrome to win over Edge", got)
	}
}

func TestLocateFindsChromeOnWindows(t *testing.T) {
	env := envOf(map[string]string{"ProgramFiles": `C:\Program Files`})
	want := `C:\Program Files\Google\Chrome\Application\chrome.exe`
	got, err := locate("windows", env, noLookPath, existsOnly(want))
	if err != nil {
		t.Fatalf("locate: %v", err)
	}
	if got != want {
		t.Errorf("locate = %q, want %q", got, want)
	}
}

func TestLocateUsesThePathOnLinux(t *testing.T) {
	lookPath := func(name string) (string, error) {
		if name == "chromium" {
			return "/usr/bin/chromium", nil
		}
		return "", errors.New("not in PATH")
	}
	got, err := locate("linux", envOf(nil), lookPath, existsOnly())
	if err != nil {
		t.Fatalf("locate: %v", err)
	}
	if got != "/usr/bin/chromium" {
		t.Errorf("locate = %q, want the chromium found in PATH", got)
	}
}

func TestLocateSaysWhatToInstallWhenNothingIsThere(t *testing.T) {
	_, err := locate("linux", envOf(nil), noLookPath, existsOnly())
	if err == nil {
		t.Fatal("locate found a browser on an empty machine")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "chrome") {
		t.Errorf("error = %q, want it to name the browser to install", err)
	}
}
