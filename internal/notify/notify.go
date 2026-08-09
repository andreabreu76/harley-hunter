package notify

import "context"

type Alert struct {
	Message string
	URL     string
}

type Notifier interface {
	Send(ctx context.Context, alert Alert) error
}
