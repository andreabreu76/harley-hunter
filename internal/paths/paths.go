package paths

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

const appName = "harley-hunter"

const overrideEnv = "HARLEY_HUNTER_HOME"

func AppDir() (string, error) {
	return appDir(runtime.GOOS, os.Getenv)
}

func appDir(goos string, env func(string) string) (string, error) {
	if custom := env(overrideEnv); custom != "" {
		return custom, nil
	}
	switch goos {
	case "darwin":
		home := env("HOME")
		if home == "" {
			return "", fmt.Errorf("no HOME in the environment: cannot tell where %s should keep its files", appName)
		}
		return filepath.Join(home, "Library", "Application Support", appName), nil
	case "windows":
		base := env("LOCALAPPDATA")
		if base == "" {
			return "", fmt.Errorf("no LOCALAPPDATA in the environment: cannot tell where %s should keep its files", appName)
		}
		return filepath.Join(base, appName), nil
	default:
		if base := env("XDG_DATA_HOME"); base != "" {
			return filepath.Join(base, appName), nil
		}
		home := env("HOME")
		if home == "" {
			return "", fmt.Errorf("no HOME in the environment: cannot tell where %s should keep its files", appName)
		}
		return filepath.Join(home, ".local", "share", appName), nil
	}
}

func ConfigFile(dir string) string { return filepath.Join(dir, "config.yaml") }

func DatabaseFile(dir string) string { return filepath.Join(dir, "hunter.db") }

func ChromeProfile(dir string) string { return filepath.Join(dir, "chrome-profile") }

func LogFile(dir string) string { return filepath.Join(dir, "logs", "hunter.log") }
