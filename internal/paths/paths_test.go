package paths

import (
	"path/filepath"
	"testing"
)

func envOf(pairs map[string]string) func(string) string {
	return func(key string) string { return pairs[key] }
}

func TestAppDirPerOperatingSystem(t *testing.T) {
	cases := []struct {
		name string
		goos string
		env  map[string]string
		want string
	}{
		{
			name: "macos uses application support",
			goos: "darwin",
			env:  map[string]string{"HOME": "/Users/rider"},
			want: filepath.Join("/Users/rider", "Library", "Application Support", "harley-hunter"),
		},
		{
			name: "windows uses local appdata and not roaming",
			goos: "windows",
			env:  map[string]string{"LOCALAPPDATA": `C:\Users\rider\AppData\Local`, "APPDATA": `C:\Users\rider\AppData\Roaming`},
			want: filepath.Join(`C:\Users\rider\AppData\Local`, "harley-hunter"),
		},
		{
			name: "linux honours xdg data home",
			goos: "linux",
			env:  map[string]string{"HOME": "/home/rider", "XDG_DATA_HOME": "/home/rider/.data"},
			want: filepath.Join("/home/rider/.data", "harley-hunter"),
		},
		{
			name: "linux falls back to local share",
			goos: "linux",
			env:  map[string]string{"HOME": "/home/rider"},
			want: filepath.Join("/home/rider", ".local", "share", "harley-hunter"),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := appDir(c.goos, envOf(c.env))
			if err != nil {
				t.Fatalf("appDir: %v", err)
			}
			if got != c.want {
				t.Errorf("appDir = %q, want %q", got, c.want)
			}
		})
	}
}

func TestAppDirOverrideWinsOnEveryOperatingSystem(t *testing.T) {
	for _, goos := range []string{"darwin", "windows", "linux"} {
		env := envOf(map[string]string{
			"HARLEY_HUNTER_HOME": "/tmp/hunter-test",
			"HOME":               "/Users/rider",
			"LOCALAPPDATA":       `C:\Users\rider\AppData\Local`,
		})
		got, err := appDir(goos, env)
		if err != nil {
			t.Fatalf("appDir on %s: %v", goos, err)
		}
		if got != "/tmp/hunter-test" {
			t.Errorf("appDir on %s = %q, want the override", goos, got)
		}
	}
}

func TestAppDirFailsWhenTheSystemHasNoHome(t *testing.T) {
	if _, err := appDir("darwin", envOf(nil)); err == nil {
		t.Fatal("appDir with no HOME returned no error")
	}
	if _, err := appDir("windows", envOf(nil)); err == nil {
		t.Fatal("appDir with no LOCALAPPDATA returned no error")
	}
}

func TestFileHelpersHangOffTheDirectory(t *testing.T) {
	dir := filepath.Join("/tmp", "hunter")
	if got, want := ConfigFile(dir), filepath.Join(dir, "config.yaml"); got != want {
		t.Errorf("ConfigFile = %q, want %q", got, want)
	}
	if got, want := DatabaseFile(dir), filepath.Join(dir, "hunter.db"); got != want {
		t.Errorf("DatabaseFile = %q, want %q", got, want)
	}
	if got, want := ChromeProfile(dir), filepath.Join(dir, "chrome-profile"); got != want {
		t.Errorf("ChromeProfile = %q, want %q", got, want)
	}
	if got, want := LogFile(dir), filepath.Join(dir, "logs", "hunter.log"); got != want {
		t.Errorf("LogFile = %q, want %q", got, want)
	}
}
