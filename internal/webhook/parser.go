package webhook

import (
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"strings"
)

type ReplyMode int

const (
	ReplyTwiML ReplyMode = iota
	ReplyJSON
)

type InboundMessage struct {
	Phone     string
	Body      string
	ReplyMode ReplyMode
}

var (
	ErrUnsupportedContentType = errors.New("unsupported content type")
	ErrMissingFields          = errors.New("missing required fields")
)

func ParseInbound(r *http.Request) (InboundMessage, error) {
	ctype := r.Header.Get("Content-Type")
	mediaType, _, err := mime.ParseMediaType(ctype)
	if err != nil {
		return InboundMessage{}, fmt.Errorf("parse content-type %q: %w", ctype, ErrUnsupportedContentType)
	}

	switch mediaType {
	case "application/x-www-form-urlencoded":
		return parseTwilio(r)
	case "application/json":
		return parseMeta(r)
	default:
		return InboundMessage{}, fmt.Errorf("%w: %s", ErrUnsupportedContentType, mediaType)
	}
}

func parseTwilio(r *http.Request) (InboundMessage, error) {
	if err := r.ParseForm(); err != nil {
		return InboundMessage{}, fmt.Errorf("parse form: %w", err)
	}
	from := strings.TrimSpace(r.PostForm.Get("From"))
	body := strings.TrimSpace(r.PostForm.Get("Body"))
	if from == "" || body == "" {
		return InboundMessage{}, fmt.Errorf("%w: From and Body required", ErrMissingFields)
	}
	return InboundMessage{
		Phone:     strings.TrimPrefix(from, "whatsapp:"),
		Body:      body,
		ReplyMode: ReplyTwiML,
	}, nil
}

type metaPayload struct {
	Entry []struct {
		Changes []struct {
			Value struct {
				Messages []struct {
					From string `json:"from"`
					Text struct {
						Body string `json:"body"`
					} `json:"text"`
				} `json:"messages"`
			} `json:"value"`
		} `json:"changes"`
	} `json:"entry"`
}

func parseMeta(r *http.Request) (InboundMessage, error) {
	var p metaPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		return InboundMessage{}, fmt.Errorf("decode json: %w", err)
	}
	if len(p.Entry) == 0 || len(p.Entry[0].Changes) == 0 || len(p.Entry[0].Changes[0].Value.Messages) == 0 {
		return InboundMessage{}, fmt.Errorf("%w: no messages in payload", ErrMissingFields)
	}
	m := p.Entry[0].Changes[0].Value.Messages[0]
	from := strings.TrimSpace(m.From)
	body := strings.TrimSpace(m.Text.Body)
	if from == "" || body == "" {
		return InboundMessage{}, fmt.Errorf("%w: from/text.body required", ErrMissingFields)
	}
	return InboundMessage{
		Phone:     strings.TrimPrefix(from, "whatsapp:"),
		Body:      body,
		ReplyMode: ReplyJSON,
	}, nil
}
