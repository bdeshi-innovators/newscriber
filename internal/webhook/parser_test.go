package webhook

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestParseInbound_Twilio(t *testing.T) {
	form := url.Values{}
	form.Set("From", "whatsapp:+391112223333")
	form.Set("Body", "Get News")

	req, err := http.NewRequest(http.MethodPost, "/webhook/whatsapp", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	msg, err := ParseInbound(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Phone != "+391112223333" {
		t.Errorf("phone: got %q want %q", msg.Phone, "+391112223333")
	}
	if msg.Body != "Get News" {
		t.Errorf("body: got %q want %q", msg.Body, "Get News")
	}
	if msg.ReplyMode != ReplyTwiML {
		t.Errorf("reply mode: got %v want %v", msg.ReplyMode, ReplyTwiML)
	}
}

func TestParseInbound_TwilioNoPrefix(t *testing.T) {
	form := url.Values{}
	form.Set("From", "+391112223333")
	form.Set("Body", "1")
	req, _ := http.NewRequest(http.MethodPost, "/webhook/whatsapp", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")

	msg, err := ParseInbound(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Phone != "+391112223333" {
		t.Errorf("phone: got %q", msg.Phone)
	}
	if msg.Body != "1" {
		t.Errorf("body: got %q", msg.Body)
	}
}

func TestParseInbound_MetaJSON(t *testing.T) {
	payload := `{
		"entry":[{
			"changes":[{
				"value":{
					"messages":[{
						"from":"391112223333",
						"text":{"body":"2"}
					}]
				}
			}]
		}]
	}`
	req, _ := http.NewRequest(http.MethodPost, "/webhook/whatsapp", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")

	msg, err := ParseInbound(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Phone != "391112223333" {
		t.Errorf("phone: got %q", msg.Phone)
	}
	if msg.Body != "2" {
		t.Errorf("body: got %q", msg.Body)
	}
	if msg.ReplyMode != ReplyJSON {
		t.Errorf("reply mode: got %v want %v", msg.ReplyMode, ReplyJSON)
	}
}

func TestParseInbound_MetaJSON_NoMessages(t *testing.T) {
	payload := `{"entry":[{"changes":[{"value":{"messages":[]}}]}]}`
	req, _ := http.NewRequest(http.MethodPost, "/webhook/whatsapp", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")

	_, err := ParseInbound(req)
	if err == nil {
		t.Fatal("expected error for empty messages array")
	}
}

func TestParseInbound_UnsupportedContentType(t *testing.T) {
	req, _ := http.NewRequest(http.MethodPost, "/webhook/whatsapp", strings.NewReader("anything"))
	req.Header.Set("Content-Type", "text/plain")

	_, err := ParseInbound(req)
	if err == nil {
		t.Fatal("expected error for unsupported content type")
	}
}

func TestParseInbound_TwilioMissingFields(t *testing.T) {
	form := url.Values{}
	form.Set("Body", "Get News")
	req, _ := http.NewRequest(http.MethodPost, "/webhook/whatsapp", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	_, err := ParseInbound(req)
	if err == nil {
		t.Fatal("expected error when From is missing")
	}
}
