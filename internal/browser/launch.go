package browser

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"time"
)

const (
	probeAttempts = 30
	probeInterval = time.Second
	probeTimeout  = 2 * time.Second
)

type Options struct {
	ExecutablePath string
	ProfileDir     string
	ExistingURL    string
	Headless       bool
}

type Handle struct {
	url   string
	cmd   *exec.Cmd
	kill  func(*exec.Cmd) error
	owned bool
}

type deps struct {
	start    func(*exec.Cmd) error
	kill     func(*exec.Cmd) error
	probe    func(url string) error
	freePort func() (int, error)
	sleep    func(time.Duration)
}

func Launch(ctx context.Context, opts Options) (*Handle, error) {
	return launch(ctx, opts, deps{
		start:    func(cmd *exec.Cmd) error { return cmd.Start() },
		kill:     func(cmd *exec.Cmd) error { return cmd.Process.Kill() },
		probe:    probeDevtools,
		freePort: freePort,
		sleep:    time.Sleep,
	})
}

func launch(ctx context.Context, opts Options, d deps) (*Handle, error) {
	if opts.ExistingURL != "" {
		if err := d.probe(opts.ExistingURL); err != nil {
			return nil, fmt.Errorf("nothing is answering at %s: start Chrome with a debugging port or clear devtools_url so the hunter starts one: %w", opts.ExistingURL, err)
		}
		return &Handle{url: opts.ExistingURL}, nil
	}

	port, err := d.freePort()
	if err != nil {
		return nil, fmt.Errorf("choosing a debugging port: %w", err)
	}
	url := fmt.Sprintf("http://127.0.0.1:%d", port)

	cmd := exec.CommandContext(ctx, opts.ExecutablePath, Flags(opts, port)...)
	if err := d.start(cmd); err != nil {
		return nil, fmt.Errorf("starting %s: %w", opts.ExecutablePath, err)
	}

	handle := &Handle{url: url, cmd: cmd, kill: d.kill, owned: true}
	for attempt := 0; attempt < probeAttempts; attempt++ {
		if err := d.probe(url); err == nil {
			return handle, nil
		}
		d.sleep(probeInterval)
	}
	handle.Close()
	return nil, fmt.Errorf("the browser never answered at %s", url)
}

func Flags(opts Options, port int) []string {
	flags := []string{
		fmt.Sprintf("--remote-debugging-port=%d", port),
		"--user-data-dir=" + opts.ProfileDir,
		"--no-first-run",
		"--no-default-browser-check",
	}
	if opts.Headless {
		flags = append(flags, "--headless=new")
	} else {
		flags = append(flags, "--window-position=-32000,-32000", "--window-size=1280,900")
	}
	return append(flags, "about:blank")
}

func HeadlessNeeded(goos string, env func(string) string) bool {
	if goos != "linux" {
		return false
	}
	return env("DISPLAY") == "" && env("WAYLAND_DISPLAY") == ""
}

func (h *Handle) DevtoolsURL() string { return h.url }

func (h *Handle) Owned() bool { return h.owned }

func (h *Handle) Close() error {
	if !h.owned || h.cmd == nil || h.kill == nil {
		return nil
	}
	return h.kill(h.cmd)
}

func probeDevtools(url string) error {
	client := &http.Client{Timeout: probeTimeout}
	response, err := client.Get(url + "/json/version")
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("the browser answered %d at %s", response.StatusCode, url)
	}
	return nil
}

func freePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}
