package webhook_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"voicescribe-webhook/internal/users"
	"voicescribe-webhook/internal/webhook"
)

func newTwilioRequest(t *testing.T, from, body string) *http.Request {
	t.Helper()
	form := url.Values{}
	form.Set("From", from)
	form.Set("Body", body)
	req := httptest.NewRequest(http.MethodPost, "/webhook/whatsapp", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

func newMetaRequest(t *testing.T, from, body string) *http.Request {
	t.Helper()
	payload := map[string]any{
		"entry": []any{map[string]any{
			"changes": []any{map[string]any{
				"value": map[string]any{
					"messages": []any{map[string]any{
						"from": from,
						"text": map[string]any{"body": body},
					}},
				},
			}},
		}},
	}
	b, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/webhook/whatsapp", strings.NewReader(string(b)))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func TestHandler_UnknownPhone_ReturnsWelcomeMenu(t *testing.T) {
	repo := users.NewInMemoryUserRepository()
	h := webhook.NewHandler(repo)

	req := newTwilioRequest(t, "whatsapp:+391112223333", "hello")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Welcome to VoiceScribe") {
		t.Errorf("expected welcome menu, got: %s", body)
	}
	if !strings.Contains(body, "1️⃣ English") {
		t.Errorf("expected English option, got: %s", body)
	}
}

func TestHandler_GetNewsMixedCase_ReturnsWelcomeMenu(t *testing.T) {
	repo := users.NewInMemoryUserRepository()
	_ = repo.UpsertUser(context.Background(), "+391112223333", "en")
	h := webhook.NewHandler(repo)

	req := newTwilioRequest(t, "whatsapp:+391112223333", "GeT NeWs")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if !strings.Contains(rr.Body.String(), "Welcome to VoiceScribe") {
		t.Errorf("expected welcome menu, got: %s", rr.Body.String())
	}
}

func TestHandler_LanguageSelection(t *testing.T) {
	cases := []struct {
		input    string
		wantLang string
	}{
		{"1", "en"},
		{"2", "it"},
		{"3", "bn"},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			repo := users.NewInMemoryUserRepository()
			h := webhook.NewHandler(repo)

			req := newTwilioRequest(t, "whatsapp:+391112223333", tc.input)
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("status: got %d", rr.Code)
			}
			body := rr.Body.String()
			if !strings.Contains(body, "set! Here is your latest update") {
				t.Errorf("expected confirmation, got: %s", body)
			}
			if !strings.Contains(body, ".mp3") {
				t.Errorf("expected MP3 URL, got: %s", body)
			}

			stored, _ := repo.GetUser(context.Background(), "+391112223333")
			if stored == nil {
				t.Fatal("expected user persisted")
			}
			if stored.LanguagePref != tc.wantLang {
				t.Errorf("language: got %q want %q", stored.LanguagePref, tc.wantLang)
			}
		})
	}
}

func TestHandler_ExistingUserFreeText_ReturnsReminder(t *testing.T) {
	repo := users.NewInMemoryUserRepository()
	_ = repo.UpsertUser(context.Background(), "+391112223333", "it")
	h := webhook.NewHandler(repo)

	req := newTwilioRequest(t, "whatsapp:+391112223333", "ciao")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	body := rr.Body.String()
	if !strings.Contains(body, "next VoiceScribe drop") {
		t.Errorf("expected reminder, got: %s", body)
	}

	stored, _ := repo.GetUser(context.Background(), "+391112223333")
	if stored.LanguagePref != "it" {
		t.Errorf("language pref should be unchanged, got %q", stored.LanguagePref)
	}
}

func TestHandler_UnsupportedContentType_Returns400(t *testing.T) {
	repo := users.NewInMemoryUserRepository()
	h := webhook.NewHandler(repo)

	req := httptest.NewRequest(http.MethodPost, "/webhook/whatsapp", strings.NewReader("anything"))
	req.Header.Set("Content-Type", "text/plain")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status: got %d want 400", rr.Code)
	}
}

func TestHandler_MetaJSONFlow(t *testing.T) {
	repo := users.NewInMemoryUserRepository()
	h := webhook.NewHandler(repo)

	req := newMetaRequest(t, "391112223333", "1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d body=%s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("content-type: got %q want application/json", ct)
	}
	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, rr.Body.String())
	}
	if !strings.Contains(resp["reply"], "You're set!") {
		t.Errorf("expected JSON reply with confirmation, got: %v", resp)
	}

	stored, _ := repo.GetUser(context.Background(), "391112223333")
	if stored == nil || stored.LanguagePref != "en" {
		t.Errorf("expected en preference stored, got %+v", stored)
	}
}

func TestHandler_TwilioReplyIsTwiML(t *testing.T) {
	repo := users.NewInMemoryUserRepository()
	h := webhook.NewHandler(repo)

	req := newTwilioRequest(t, "whatsapp:+391112223333", "1")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/xml") {
		t.Errorf("content-type: got %q want text/xml", ct)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "<Response>") || !strings.Contains(body, "<Message>") {
		t.Errorf("expected TwiML envelope, got: %s", body)
	}
}
