package alert

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSlackAlerter_Name(t *testing.T) {
	a := NewSlackAlerter("https://hooks.slack.com/services/T/B/x", "")
	if a.Name() != "slack" {
		t.Errorf("name = %q, want slack", a.Name())
	}
}

func TestSlackAlerter_Success(t *testing.T) {
	var received slackPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("content-type = %q", r.Header.Get("Content-Type"))
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decoding payload: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	alerter := NewSlackAlerter(server.URL, "")
	err := alerter.Send(context.Background(), testEvent())
	if err != nil {
		t.Fatal(err)
	}

	// Verify fallback text
	if received.Text == "" {
		t.Error("fallback text should not be empty")
	}

	// Verify attachment structure
	if len(received.Attachments) != 1 {
		t.Fatalf("attachments = %d, want 1", len(received.Attachments))
	}
	att := received.Attachments[0]
	if att.Color != "#ECB22E" {
		t.Errorf("color = %q, want #ECB22E (warning)", att.Color)
	}

	// Expect 4 blocks: header, fields, message, context
	if len(att.Blocks) != 4 {
		t.Errorf("blocks = %d, want 4", len(att.Blocks))
	}
}

func TestSlackAlerter_ChannelOverride(t *testing.T) {
	var received slackPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decoding payload: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	alerter := NewSlackAlerter(server.URL, "#alerts")
	if err := alerter.Send(context.Background(), testEvent()); err != nil {
		t.Fatal(err)
	}

	if received.Channel != "#alerts" {
		t.Errorf("channel = %q, want #alerts", received.Channel)
	}
}

func TestSlackAlerter_NoChannelWhenEmpty(t *testing.T) {
	var raw map[string]json.RawMessage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			t.Fatalf("decoding payload: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	alerter := NewSlackAlerter(server.URL, "")
	if err := alerter.Send(context.Background(), testEvent()); err != nil {
		t.Fatal(err)
	}

	if _, ok := raw["channel"]; ok {
		t.Error("channel should be omitted when empty")
	}
}

func TestSlackAlerter_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	alerter := NewSlackAlerter(server.URL, "")
	err := alerter.Send(context.Background(), testEvent())
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

func TestSlackAlerter_RateLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	alerter := NewSlackAlerter(server.URL, "")
	err := alerter.Send(context.Background(), testEvent())
	if err == nil {
		t.Error("expected error for 429 response")
	}
}

func TestSlackAlerter_SeverityColors(t *testing.T) {
	tests := []struct {
		severity string
		color    string
	}{
		{"critical", "#E01E5A"},
		{"expired", "#E01E5A"},
		{"warning", "#ECB22E"},
		{"ok", "#2EB886"},
		{"unknown", "#CCCCCC"},
	}

	for _, tt := range tests {
		t.Run(tt.severity, func(t *testing.T) {
			got := severityColor(tt.severity)
			if got != tt.color {
				t.Errorf("severityColor(%q) = %q, want %q", tt.severity, got, tt.color)
			}
		})
	}
}

func TestSlackAlerter_WithImpact(t *testing.T) {
	var received slackPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decoding payload: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	event := Event{
		Source:    "aib",
		EventType: "cert_expiring",
		Severity:  "critical",
		Asset: Asset{
			ID:            "probe:certificate:api.example.com",
			Name:          "api.example.com",
			Type:          "certificate",
			DaysRemaining: 3,
		},
		Impact: &Impact{
			AffectedCount:    5,
			AffectedServices: []string{"web-frontend", "api-gateway"},
		},
		Message:   "Certificate expiring in 3 days",
		Timestamp: time.Now(),
	}

	alerter := NewSlackAlerter(server.URL, "")
	if err := alerter.Send(context.Background(), event); err != nil {
		t.Fatal(err)
	}

	att := received.Attachments[0]
	if att.Color != "#E01E5A" {
		t.Errorf("color = %q, want #E01E5A (critical)", att.Color)
	}

	// Expect 5 blocks: header, fields, message, impact, context
	if len(att.Blocks) != 5 {
		t.Errorf("blocks = %d, want 5 (with impact)", len(att.Blocks))
	}
}
