package notify

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

type recordedCommand struct {
	ctx  context.Context
	name string
	args []string
}

func recorder() (*[]recordedCommand, func(ctx context.Context, name string, args ...string) error) {
	var calls []recordedCommand
	return &calls, func(ctx context.Context, name string, args ...string) error {
		calls = append(calls, recordedCommand{ctx: ctx, name: name, args: args})
		return nil
	}
}

func sendVia(t *testing.T, binaryPath string, alert Alert) recordedCommand {
	t.Helper()
	calls, run := recorder()
	n := NewMacOS()
	n.binaryPath = binaryPath
	n.run = run

	if err := n.Send(context.Background(), alert); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(*calls) != 1 {
		t.Fatalf("ran %d commands, want 1", len(*calls))
	}
	return (*calls)[0]
}

func TestMacOSSendsThroughTerminalNotifierWhenPresent(t *testing.T) {
	alert := Alert{
		Message: "Harley-Davidson Street Glide 2014 - R$ 72.000 - curitiba/PR [olx]",
		URL:     "https://pr.olx.com.br/regiao-de-curitiba/motos/harley-1509210244",
	}
	got := sendVia(t, "/opt/homebrew/bin/terminal-notifier", alert)

	if got.name != "/opt/homebrew/bin/terminal-notifier" {
		t.Errorf("command = %q, want the cached terminal-notifier path", got.name)
	}
	want := []string{
		"-title", "Harley Hunter",
		"-message", alert.Message,
		"-open", alert.URL,
		"-sound", "Glass",
	}
	if !reflect.DeepEqual(got.args, want) {
		t.Errorf("args =\n%q\nwant\n%q", got.args, want)
	}
}

func TestMacOSPassesTheMessageUntouchedToTerminalNotifier(t *testing.T) {
	raw := `Harley "Street Glide" 2014 \ back - R$ 72.000`
	got := sendVia(t, "/opt/homebrew/bin/terminal-notifier", Alert{Message: raw, URL: "https://olx.com.br/abc"})

	for i, a := range got.args {
		if a == "-message" {
			if got.args[i+1] != raw {
				t.Errorf("message = %q, want it verbatim: exec takes an argument vector, so escaping would corrupt it", got.args[i+1])
			}
			return
		}
	}
	t.Fatal("no -message argument")
}

func TestMacOSOmitsOpenWhenTheAlertHasNoURL(t *testing.T) {
	got := sendVia(t, "/opt/homebrew/bin/terminal-notifier", Alert{Message: "sem link"})

	for _, a := range got.args {
		if a == "-open" {
			t.Errorf("args %q pass -open with nothing to open", got.args)
		}
	}
}

func TestMacOSFallsBackToOsascriptWithoutTheBinary(t *testing.T) {
	got := sendVia(t, "", Alert{Message: "Street Glide 2014", URL: "https://olx.com.br/abc"})

	if got.name != "osascript" {
		t.Fatalf("command = %q, want osascript as the fallback", got.name)
	}
	want := `display notification "Street Glide 2014" with title "Harley Hunter" sound name "Glass"`
	if len(got.args) != 2 || got.args[0] != "-e" || got.args[1] != want {
		t.Errorf("args =\n%q\nwant -e followed by\n%s", got.args, want)
	}
}

func TestMacOSFallbackStillEscapesTheMessage(t *testing.T) {
	got := sendVia(t, "", Alert{Message: `back\slash and "quote"`})

	want := `display notification "back\\slash and \"quote\"" with title "Harley Hunter" sound name "Glass"`
	if got.args[1] != want {
		t.Errorf("script =\n%s\nwant\n%s", got.args[1], want)
	}
}

func TestMacOSFallbackKeepsAppleScriptOutOfTheMessage(t *testing.T) {
	got := sendVia(t, "", Alert{Message: `" & (do shell script "touch /tmp/pwned") & "`})

	body := strings.TrimSuffix(strings.TrimPrefix(got.args[1], `display notification "`), `" with title "Harley Hunter" sound name "Glass"`)
	if strings.Contains(strings.ReplaceAll(body, `\"`, ""), `"`) {
		t.Errorf("body %q still carries an unescaped quote, so the message can close the literal and run code", body)
	}
}

func TestMacOSFallbackFlattensLineBreaks(t *testing.T) {
	got := sendVia(t, "", Alert{Message: "first line\nsecond\rthird"})

	if strings.ContainsAny(got.args[1], "\n\r") {
		t.Errorf("script %q carries a raw line break, which osascript rejects as a syntax error", got.args[1])
	}
}

func TestMacOSSendReportsRunnerFailure(t *testing.T) {
	n := NewMacOS()
	n.run = func(ctx context.Context, name string, args ...string) error {
		return errors.New("no gui session")
	}

	err := n.Send(context.Background(), Alert{Message: "teste"})
	if err == nil {
		t.Fatal("Send should surface a failure so the round can retry next time")
	}
	if !strings.Contains(err.Error(), "no gui session") {
		t.Errorf("error %q loses the underlying cause", err)
	}
}

func TestMacOSSendPassesTheContextThrough(t *testing.T) {
	calls, run := recorder()
	n := NewMacOS()
	n.run = run

	type key struct{}
	ctx := context.WithValue(context.Background(), key{}, "carried")
	if err := n.Send(ctx, Alert{Message: "teste"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if (*calls)[0].ctx.Value(key{}) != "carried" {
		t.Error("Send must hand its context to the command so a cancelled round stops the notifier")
	}
}

func TestNewMacOSRunsARealCommandByDefault(t *testing.T) {
	if NewMacOS().run == nil {
		t.Fatal("NewMacOS must ship a runner, otherwise Send panics outside tests")
	}
}

func TestNewMacOSLooksTheBinaryUpOnce(t *testing.T) {
	lookups := 0
	n := newMacOS(func(string) (string, error) {
		lookups++
		return "/opt/homebrew/bin/terminal-notifier", nil
	}, io.Discard)
	_, run := recorder()
	n.run = run

	for i := 0; i < 3; i++ {
		if err := n.Send(context.Background(), Alert{Message: "teste"}); err != nil {
			t.Fatalf("Send: %v", err)
		}
	}
	if lookups != 1 {
		t.Errorf("looked the binary up %d times, want 1 cached at construction", lookups)
	}
}

func TestNewMacOSFallsBackWhenLookupFails(t *testing.T) {
	n := newMacOS(func(string) (string, error) { return "", errors.New("not found") }, io.Discard)
	if n.binaryPath != "" {
		t.Errorf("binaryPath = %q, want empty so Send takes the osascript path", n.binaryPath)
	}
}

func TestNewMacOSWarnsWhenTheBinaryIsMissing(t *testing.T) {
	var warn bytes.Buffer
	newMacOS(func(string) (string, error) { return "", errors.New("not found") }, &warn)

	got := warn.String()
	if !strings.Contains(got, notifierBin) {
		t.Fatalf("warning = %q, want it to name %s: a scheduled run degrades silently otherwise", got, notifierBin)
	}
	if !strings.Contains(got, "PATH=") {
		t.Errorf("warning = %q, want the PATH in it: launchd hands over a minimal one and that is the usual cause", got)
	}
}

func TestNewMacOSStaysQuietWhenTheBinaryIsPresent(t *testing.T) {
	var warn bytes.Buffer
	newMacOS(func(string) (string, error) { return "/opt/homebrew/bin/terminal-notifier", nil }, &warn)

	if warn.Len() != 0 {
		t.Errorf("warning = %q, want nothing on the healthy path", warn.String())
	}
}
