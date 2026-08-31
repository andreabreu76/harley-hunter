package notify

import (
	"context"
	"runtime"
)

type Alert struct {
	Message string
	URL     string
}

type Notifier interface {
	Send(ctx context.Context, alert Alert) error
}

func New() Notifier { return newFor(runtime.GOOS) }

func newFor(goos string) Notifier {
	switch goos {
	case "darwin":
		return NewMacOS()
	case "windows":
		return NewWindows()
	default:
		return NewLinux()
	}
}
