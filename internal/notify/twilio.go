package notify

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/andreabreu76/harley-hunter/internal/format"
	"github.com/andreabreu76/harley-hunter/internal/store"
)

const (
	ChannelWhatsApp = "whatsapp"
	ChannelSMS      = "sms"
)

const whatsAppPrefix = "whatsapp:"

type Twilio struct {
	endpoint string
	channel  string
	sid      string
	token    string
	from     string
	to       string
	client   *http.Client
}

func NewTwilioFromEnv() (*Twilio, error) {
	channel := strings.ToLower(strings.TrimSpace(os.Getenv("ALERT_CHANNEL")))
	if channel == "" {
		channel = ChannelWhatsApp
	}

	sid := os.Getenv("TWILIO_ACCOUNT_SID")
	token := os.Getenv("TWILIO_AUTH_TOKEN")
	to := os.Getenv("ALERT_TO")

	var from, fromName string
	switch channel {
	case ChannelWhatsApp:
		from, fromName = os.Getenv("TWILIO_WHATSAPP_FROM"), "TWILIO_WHATSAPP_FROM"
	case ChannelSMS:
		from, fromName = os.Getenv("TWILIO_PHONE_NUMBER"), "TWILIO_PHONE_NUMBER"
		if strings.TrimSpace(from) == "" {
			from = os.Getenv("TWILIO_FROM")
		}
	default:
		return nil, fmt.Errorf("ALERT_CHANNEL %q is not deliverable: use %q or %q", channel, ChannelWhatsApp, ChannelSMS)
	}

	required := []struct {
		name  string
		value string
	}{
		{"TWILIO_ACCOUNT_SID", sid},
		{"TWILIO_AUTH_TOKEN", token},
		{fromName, from},
		{"ALERT_TO", to},
	}
	var missing []string
	for _, r := range required {
		if strings.TrimSpace(r.value) == "" {
			missing = append(missing, r.name)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing environment variables for the %s channel: %s", channel, strings.Join(missing, ", "))
	}

	from, to = strings.TrimSpace(from), strings.TrimSpace(to)
	if channel == ChannelWhatsApp {
		from, to = withWhatsAppPrefix(from), withWhatsAppPrefix(to)
	}

	return &Twilio{
		endpoint: fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s/Messages.json", url.PathEscape(sid)),
		channel:  channel,
		sid:      sid,
		token:    token,
		from:     from,
		to:       to,
		client:   &http.Client{Timeout: 20 * time.Second},
	}, nil
}

func withWhatsAppPrefix(number string) string {
	if strings.HasPrefix(number, whatsAppPrefix) {
		return number
	}
	return whatsAppPrefix + number
}

func (t *Twilio) Channel() string {
	return t.channel
}

func (t *Twilio) Send(ctx context.Context, message string) error {
	form := url.Values{}
	form.Set("From", t.from)
	form.Set("To", t.to)
	form.Set("Body", message)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("building twilio request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if t.sid != "" {
		req.SetBasicAuth(t.sid, t.token)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("calling twilio: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("twilio returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func FormatAlert(r store.Row) string {
	price := "preço não informado"
	if r.PriceCents != nil {
		price = "R$ " + format.Thousands(*r.PriceCents/100)
	}
	title := strings.Join(strings.Fields(r.Title), " ")
	year := ""
	if r.Year != nil {
		if stamp := strconv.Itoa(*r.Year); !strings.Contains(title, stamp) {
			year = " " + stamp
		}
	}
	location := r.City
	if r.State != "" {
		if location == "" {
			location = r.State
		} else {
			location += "/" + r.State
		}
	}

	fields := []string{strings.TrimSpace(title + year), price}
	if location != "" {
		fields = append(fields, location)
	}
	return fmt.Sprintf("%s [%s] %s", strings.Join(fields, " - "), r.Source, r.URL)
}
