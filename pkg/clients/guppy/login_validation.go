package guppy

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/storacha/smelt/pkg/clients/smtp4dev"
)

type clickerConfig struct {
	doer smtp4dev.HTTPDoer
}

type SMTP4DevLoginValidatorOption func(*clickerConfig)

// WithSMTP4DevLoginValidatorDoer configures the HTTP doer used for both
// smtp4dev API calls and POSTing the validation link. Defaults to
// [http.DefaultClient].
func WithSMTP4DevLoginValidatorDoer(doer smtp4dev.HTTPDoer) SMTP4DevLoginValidatorOption {
	return func(c *clickerConfig) {
		c.doer = doer
	}
}

type SMTP4DevLoginValidator struct {
	Client *smtp4dev.Client
	Doer   smtp4dev.HTTPDoer
}

// NewSMTP4DevLoginValidator returns a new [SMTP4DevLoginValidator] that can
// be used to validate logins by clicking links in emails sent to a SMTP4Dev
// server.
func NewSMTP4DevLoginValidator(endpoint string, options ...SMTP4DevLoginValidatorOption) (*SMTP4DevLoginValidator, error) {
	cfg := clickerConfig{doer: http.DefaultClient}
	for _, option := range options {
		option(&cfg)
	}
	client, err := smtp4dev.New(endpoint, smtp4dev.WithDoer(cfg.doer))
	if err != nil {
		return nil, err
	}
	return &SMTP4DevLoginValidator{
		Client: client,
		Doer:   cfg.doer,
	}, nil
}

// ValidateEmailLogin polls the SMTP4Dev server for an email sent to the given
// address, extracts the validation link, and clicks it. Use the passed context
// to cancel the polling.
//
// Note: If the client is already logged in, no email link will be sent, so it
// is important to always cancel the context or this method may never return.
func (ec *SMTP4DevLoginValidator) ValidateEmailLogin(ctx context.Context, email string) error {
	found := false
	for !found {
		select {
		case <-time.After(100 * time.Millisecond):
		case <-ctx.Done():
			return ctx.Err()
		}

		page := 0
		for !found {
			msgPage, err := ec.Client.Messages(ctx, smtp4dev.WithPage(page))
			if err != nil {
				return fmt.Errorf("fetching messages: %w", err)
			}
			for _, msg := range msgPage.Results {
				if msg.DeliveredTo != email {
					continue
				}
				body, err := ec.Client.MessageBodyPlainText(ctx, msg.ID)
				if err != nil {
					return fmt.Errorf("fetching message body: %w", err)
				}
				link, err := extractValidationLink(body)
				if err != nil {
					continue
				}
				req, err := http.NewRequestWithContext(ctx, http.MethodPost, link.String(), nil)
				if err != nil {
					return fmt.Errorf("creating validation request: %w", err)
				}
				req.Header.Set("Content-Type", "text/plain")
				res, err := ec.Doer.Do(req)
				if err != nil {
					return fmt.Errorf("clicking validation link: %w", err)
				}
				res.Body.Close()
				if res.StatusCode < 200 || res.StatusCode >= 300 {
					return fmt.Errorf("clicking validation link: received status code %d", res.StatusCode)
				}
				// clean up the message
				err = ec.Client.DeleteMessage(ctx, msg.ID)
				if err != nil {
					return fmt.Errorf("deleting message: %w", err)
				}
				found = true
				break
			}

			if page >= msgPage.PageCount-1 {
				break
			}
			page++
		}
	}
	if !found {
		return fmt.Errorf("validation email not found for: %s", email)
	}
	return nil
}

// TODO: this is pretty brittle - it assumes the whole body is the link, which
// is not necessarily the case. Probably needs a regex.
func extractValidationLink(body string) (url.URL, error) {
	if !strings.Contains(body, "validate-email") {
		return url.URL{}, fmt.Errorf("validation link not found")
	}
	u, err := url.Parse(strings.TrimSpace(body))
	if err != nil {
		return url.URL{}, fmt.Errorf("parsing validation link: %w", err)
	}
	return *u, nil
}
