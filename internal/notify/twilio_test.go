package notify

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/andreabreu76/harley-hunter/internal/store"
)

func TestFormatAlertIncludesEssentials(t *testing.T) {
	year := 2015
	cents := int64(7200000)
	row := store.Row{
		Title:      "Harley Davidson Street Glide Special",
		Year:       &year,
		PriceCents: &cents,
		City:       "curitiba",
		State:      "PR",
		URL:        "https://olx.com.br/abc",
		Source:     "olx",
	}
	msg := FormatAlert(row)

	for _, want := range []string{"Street Glide", "2015", "72.000", "curitiba", "https://olx.com.br/abc"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message %q is missing %q", msg, want)
		}
	}
	if len(msg) > 320 {
		t.Errorf("message is %d chars, which spans too many SMS segments", len(msg))
	}
}

const gsm7Alphabet = "@£$¥èéùìòÇ\nØø\rÅåΔ_ΦΓΛΩΠΨΣΘΞÆæßÉ !\"#¤%&'()*+,-./0123456789:;<=>?¡" +
	"ABCDEFGHIJKLMNOPQRSTUVWXYZÄÖÑÜ§¿abcdefghijklmnopqrstuvwxyzäöñüà^{}\\[~]|€"

func TestFormatAlertStaysInTheGSM7Alphabet(t *testing.T) {
	year := 2014
	cents := int64(7190000)
	row := store.Row{
		Title:      "HARLEY DAVDSON FLHX STREET GLIDE AZUL -2014 90.195Km",
		Year:       &year,
		PriceCents: &cents,
		City:       "curitiba",
		State:      "PR",
		URL:        "https://pr.olx.com.br/regiao-de-curitiba-e-paranagua/autos-e-pecas/motos/harley-1518392408",
		Source:     "olx",
	}
	msg := FormatAlert(row)

	for _, r := range msg {
		if !strings.ContainsRune(gsm7Alphabet, r) {
			t.Errorf("message %q carries %q, outside GSM-7, which forces every segment into UCS-2", msg, r)
		}
	}
}

func TestFormatAlertDoesNotRepeatAYearAlreadyInTheTitle(t *testing.T) {
	year := 2014
	cents := int64(7200000)
	row := store.Row{
		Title:      "Harley-Davidson Street Glide 2014",
		Year:       &year,
		PriceCents: &cents,
		City:       "curitiba",
		State:      "PR",
		URL:        "https://olx.com.br/abc",
		Source:     "olx",
	}
	msg := FormatAlert(row)

	if strings.Contains(msg, "2014 2014") {
		t.Errorf("message %q repeats the year already present in the title", msg)
	}
	if !strings.Contains(msg, "2014") {
		t.Errorf("message %q lost the year entirely", msg)
	}
}

func TestFormatAlertCollapsesRepeatedSpacesInTheTitle(t *testing.T) {
	year := 2014
	cents := int64(7190000)
	row := store.Row{
		Title:      "HARLEY DAVDSON FLHX STREET GLIDE  AZUL -2014 90.195Km",
		Year:       &year,
		PriceCents: &cents,
		City:       "curitiba",
		State:      "PR",
		URL:        "https://olx.com.br/abc",
		Source:     "olx",
	}
	msg := FormatAlert(row)

	if strings.Contains(msg, "  ") {
		t.Errorf("message %q keeps the double space the listing title carried", msg)
	}
}

func TestFormatAlertWithoutPriceOrYear(t *testing.T) {
	row := store.Row{
		Title:  "Harley Davidson Road Glide",
		City:   "sao paulo",
		State:  "SP",
		URL:    "https://mercadolivre.com.br/xyz",
		Source: "mercadolivre",
	}
	msg := FormatAlert(row)

	if !strings.Contains(msg, "preço não informado") {
		t.Errorf("message %q should say the price is missing", msg)
	}
	if strings.Contains(msg, "  ") {
		t.Errorf("message %q has a double space left by the absent year", msg)
	}
	for _, want := range []string{"Road Glide", "sao paulo/SP", "https://mercadolivre.com.br/xyz"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message %q is missing %q", msg, want)
		}
	}
}

func TestFormatAlertWithoutLocation(t *testing.T) {
	year := 2014
	cents := int64(6500000)
	row := store.Row{
		Title:      "Harley Davidson Street Glide",
		Year:       &year,
		PriceCents: &cents,
		URL:        "https://olx.com.br/abc",
		Source:     "olx",
	}
	msg := FormatAlert(row)

	if strings.Contains(msg, "  ") {
		t.Errorf("message %q has a blank gap left by the absent location", msg)
	}
	if strings.Contains(msg, "- -") {
		t.Errorf("message %q has an empty field between separators", msg)
	}
	if !strings.Contains(msg, "https://olx.com.br/abc") {
		t.Errorf("message %q lost the url", msg)
	}
}

func TestFormatAlertWithStateOnly(t *testing.T) {
	year := 2014
	cents := int64(6500000)
	row := store.Row{
		Title:      "Harley Davidson Street Glide",
		Year:       &year,
		PriceCents: &cents,
		State:      "PR",
		URL:        "https://olx.com.br/abc",
		Source:     "olx",
	}
	msg := FormatAlert(row)

	if strings.Contains(msg, " /PR") {
		t.Errorf("message %q starts the location with a bare separator", msg)
	}
	if !strings.Contains(msg, "PR") {
		t.Errorf("message %q lost the state", msg)
	}
}

func TestTwilioSendPostsToAPI(t *testing.T) {
	var gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
		}
		gotBody = r.Form.Get("Body")
		if r.Form.Get("To") != "+5541999999999" {
			t.Errorf("To = %q", r.Form.Get("To"))
		}
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"sid":"SM123"}`))
	}))
	defer server.Close()

	tw := &Twilio{
		endpoint: server.URL,
		from:     "+15550001111",
		to:       "+5541999999999",
		client:   &http.Client{Timeout: 5 * time.Second},
	}
	if err := tw.Send(context.Background(), "teste"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if gotBody != "teste" {
		t.Errorf("Body = %q, want %q", gotBody, "teste")
	}
}

func TestTwilioSendAuthenticates(t *testing.T) {
	var gotUser, gotPass string
	var ok bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser, gotPass, ok = r.BasicAuth()
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	tw := &Twilio{
		endpoint: server.URL,
		sid:      "AC0000",
		token:    "secret",
		from:     "+1",
		to:       "+2",
		client:   server.Client(),
	}
	if err := tw.Send(context.Background(), "teste"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if !ok || gotUser != "AC0000" || gotPass != "secret" {
		t.Errorf("basic auth = (%q, %q, %v), want the account sid and token", gotUser, gotPass, ok)
	}
}

func TestTwilioSendReportsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"message":"authenticate"}`))
	}))
	defer server.Close()

	tw := &Twilio{endpoint: server.URL, from: "+1", to: "+2", client: server.Client()}
	if err := tw.Send(context.Background(), "teste"); err == nil {
		t.Fatal("Send should return an error on a non-2xx response")
	}
}

func TestTwilioSendDoesNotLeakTokenInError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"message":"forbidden"}`))
	}))
	defer server.Close()

	tw := &Twilio{endpoint: server.URL, sid: "AC0000", token: "supersecret", from: "+1", to: "+2", client: server.Client()}
	err := tw.Send(context.Background(), "teste")
	if err == nil {
		t.Fatal("Send should return an error on a non-2xx response")
	}
	if strings.Contains(err.Error(), "supersecret") {
		t.Errorf("error %q leaks the auth token", err)
	}
}

func setCredentials(t *testing.T) {
	t.Helper()
	t.Setenv("TWILIO_ACCOUNT_SID", "AC0000")
	t.Setenv("TWILIO_AUTH_TOKEN", "token")
	t.Setenv("TWILIO_PHONE_NUMBER", "+15550001111")
	t.Setenv("TWILIO_FROM", "")
	t.Setenv("TWILIO_WHATSAPP_FROM", "+14155238886")
	t.Setenv("ALERT_TO", "+5541999999999")
	t.Setenv("ALERT_CHANNEL", "")
}

func capturedForm(t *testing.T, configure func()) url.Values {
	t.Helper()
	var got url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
		}
		got = r.Form
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	configure()
	tw, err := NewTwilioFromEnv()
	if err != nil {
		t.Fatalf("NewTwilioFromEnv: %v", err)
	}
	tw.endpoint = server.URL
	tw.client = server.Client()
	if err := tw.Send(context.Background(), "teste"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	return got
}

func TestNewTwilioFromEnvDefaultsToWhatsApp(t *testing.T) {
	setCredentials(t)

	tw, err := NewTwilioFromEnv()
	if err != nil {
		t.Fatalf("NewTwilioFromEnv: %v", err)
	}
	if tw.Channel() != ChannelWhatsApp {
		t.Errorf("channel = %q, want %q when ALERT_CHANNEL is unset", tw.Channel(), ChannelWhatsApp)
	}
}

func TestTwilioSendPrefixesWhatsAppNumbers(t *testing.T) {
	form := capturedForm(t, func() { setCredentials(t) })

	if form.Get("From") != "whatsapp:+14155238886" {
		t.Errorf("From = %q, want the whatsapp sender prefixed", form.Get("From"))
	}
	if form.Get("To") != "whatsapp:+5541999999999" {
		t.Errorf("To = %q, want the destination prefixed", form.Get("To"))
	}
}

func TestTwilioSendKeepsBareNumbersOnSMS(t *testing.T) {
	form := capturedForm(t, func() {
		setCredentials(t)
		t.Setenv("ALERT_CHANNEL", "sms")
	})

	if form.Get("From") != "+15550001111" {
		t.Errorf("From = %q, want the bare phone number on sms", form.Get("From"))
	}
	if form.Get("To") != "+5541999999999" {
		t.Errorf("To = %q, want the bare destination on sms", form.Get("To"))
	}
}

func TestNewTwilioFromEnvDoesNotDoublePrefix(t *testing.T) {
	form := capturedForm(t, func() {
		setCredentials(t)
		t.Setenv("TWILIO_WHATSAPP_FROM", "whatsapp:+14155238886")
		t.Setenv("ALERT_TO", "whatsapp:+5541999999999")
	})

	if form.Get("From") != "whatsapp:+14155238886" {
		t.Errorf("From = %q, want a single whatsapp prefix", form.Get("From"))
	}
	if form.Get("To") != "whatsapp:+5541999999999" {
		t.Errorf("To = %q, want a single whatsapp prefix", form.Get("To"))
	}
}

func TestNewTwilioFromEnvNamesTheWhatsAppSenderWhenMissing(t *testing.T) {
	setCredentials(t)
	t.Setenv("TWILIO_WHATSAPP_FROM", "")

	_, err := NewTwilioFromEnv()
	if err == nil {
		t.Fatal("NewTwilioFromEnv should fail without a whatsapp sender")
	}
	if !strings.Contains(err.Error(), "TWILIO_WHATSAPP_FROM") {
		t.Errorf("error %q does not name the variable the whatsapp channel needs", err)
	}
	if strings.Contains(err.Error(), "TWILIO_PHONE_NUMBER") {
		t.Errorf("error %q blames a variable the whatsapp channel does not use", err)
	}
}

func TestNewTwilioFromEnvNamesThePhoneSenderWhenMissingOnSMS(t *testing.T) {
	setCredentials(t)
	t.Setenv("ALERT_CHANNEL", "sms")
	t.Setenv("TWILIO_PHONE_NUMBER", "")
	t.Setenv("TWILIO_WHATSAPP_FROM", "")

	_, err := NewTwilioFromEnv()
	if err == nil {
		t.Fatal("NewTwilioFromEnv should fail without an sms sender")
	}
	if !strings.Contains(err.Error(), "TWILIO_PHONE_NUMBER") {
		t.Errorf("error %q does not name the variable the sms channel needs", err)
	}
	if strings.Contains(err.Error(), "TWILIO_WHATSAPP_FROM") {
		t.Errorf("error %q blames a variable the sms channel does not use", err)
	}
}

func TestNewTwilioFromEnvRejectsAnUnknownChannel(t *testing.T) {
	setCredentials(t)
	t.Setenv("ALERT_CHANNEL", "telegram")

	if _, err := NewTwilioFromEnv(); err == nil {
		t.Fatal("NewTwilioFromEnv should reject a channel it cannot deliver")
	} else if !strings.Contains(err.Error(), "telegram") {
		t.Errorf("error %q does not quote the unusable channel", err)
	}
}

func TestNewTwilioFromEnvListsEveryMissingVariable(t *testing.T) {
	t.Setenv("TWILIO_ACCOUNT_SID", "")
	t.Setenv("TWILIO_AUTH_TOKEN", "")
	t.Setenv("TWILIO_WHATSAPP_FROM", "")
	t.Setenv("ALERT_TO", "")
	t.Setenv("ALERT_CHANNEL", "")

	_, err := NewTwilioFromEnv()
	if err == nil {
		t.Fatal("NewTwilioFromEnv should fail when nothing is configured")
	}
	for _, want := range []string{"TWILIO_ACCOUNT_SID", "TWILIO_AUTH_TOKEN", "TWILIO_WHATSAPP_FROM", "ALERT_TO"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestNewTwilioFromEnvAcceptsTwilioFromFallback(t *testing.T) {
	setCredentials(t)
	t.Setenv("ALERT_CHANNEL", "sms")
	t.Setenv("TWILIO_PHONE_NUMBER", "")
	t.Setenv("TWILIO_FROM", "+15550001111")

	tw, err := NewTwilioFromEnv()
	if err != nil {
		t.Fatalf("NewTwilioFromEnv: %v", err)
	}
	if tw.from != "+15550001111" {
		t.Errorf("from = %q, want the TWILIO_FROM fallback", tw.from)
	}
	if !strings.Contains(tw.endpoint, "AC0000") {
		t.Errorf("endpoint = %q, want the account sid in the path", tw.endpoint)
	}
}
