package notify

import "testing"

func TestNewForPicksTheNotifierOfEachSystem(t *testing.T) {
	if _, ok := newFor("darwin").(*MacOS); !ok {
		t.Error("darwin did not get the macOS notifier")
	}
	if _, ok := newFor("windows").(*Windows); !ok {
		t.Error("windows did not get the Windows notifier")
	}
	if _, ok := newFor("linux").(*Linux); !ok {
		t.Error("linux did not get the Linux notifier")
	}
	if _, ok := newFor("freebsd").(*Linux); !ok {
		t.Error("an unknown system did not fall back to notify-send")
	}
}
