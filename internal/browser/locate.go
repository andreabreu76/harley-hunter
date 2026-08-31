package browser

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

func Locate() (string, error) {
	return locate(runtime.GOOS, os.Getenv, exec.LookPath, func(path string) bool {
		info, err := os.Stat(path)
		return err == nil && !info.IsDir()
	})
}

func locate(goos string, env func(string) string, lookPath func(string) (string, error), exists func(string) bool) (string, error) {
	for _, candidate := range candidates(goos, env) {
		if exists(candidate) {
			return candidate, nil
		}
	}
	for _, name := range pathNames(goos) {
		if found, err := lookPath(name); err == nil {
			return found, nil
		}
	}
	return "", fmt.Errorf("no browser found: install Google Chrome, Chromium or Microsoft Edge and start the hunter again")
}

func candidates(goos string, env func(string) string) []string {
	switch goos {
	case "darwin":
		return []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
		}
	case "windows":
		var paths []string
		for _, base := range []string{env("ProgramFiles"), env("ProgramFiles(x86)"), env("LOCALAPPDATA")} {
			if base == "" {
				continue
			}
			paths = append(paths,
				windowsJoin(base, "Google", "Chrome", "Application", "chrome.exe"),
				windowsJoin(base, "Chromium", "Application", "chrome.exe"),
				windowsJoin(base, "Microsoft", "Edge", "Application", "msedge.exe"),
			)
		}
		return paths
	default:
		return nil
	}
}

func windowsJoin(parts ...string) string {
	return strings.Join(parts, `\`)
}

func pathNames(goos string) []string {
	if goos == "windows" {
		return nil
	}
	return []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "microsoft-edge"}
}
