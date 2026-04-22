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

// Clicker issues the POST against the validation link pulled out of the email
// body. It is split from the smtp4dev API client because the two live on
// different planes: the API is fetched from the host via smtp4dev's mapped
// port, whereas the validation link — once sprue's public_url is in-network
// (see pkg/stack/ports.go) — only resolves from inside the Docker network.
type Clicker interface {
	Do(req *http.Request) (*http.Response, error)
}

type clickerConfig struct {
	httpClient *http.Client
	clicker    Clicker
}

type SMTP4DevLoginValidatorOption func(*clickerConfig)

// WithSMTP4DevLoginValidatorHTTPClient sets the HTTP client used to hit the
// smtp4dev API. Defaults to [http.DefaultClient].
func WithSMTP4DevLoginValidatorHTTPClient(httpClient *http.Client) SMTP4DevLoginValidatorOption {
	return func(c *clickerConfig) {
		c.httpClient = httpClient
	}
}

// WithSMTP4DevLoginValidatorClicker sets the doer used to POST the validation
// link parsed out of the email body. Defaults to [http.DefaultClient]; pass
// an in-network doer (e.g. [ExecDoer]) when the validation URL is only
// reachable from inside the Docker network.
func WithSMTP4DevLoginValidatorClicker(clicker Clicker) SMTP4DevLoginValidatorOption {
	return func(c *clickerConfig) {
		c.clicker = clicker
	}
}

type SMTP4DevLoginValidator struct {
	Client     *smtp4dev.Client
	HTTPClient *http.Client
	Clicker    Clicker
}

// NewSMTP4DevLoginValidator returns a new [SMTP4DevLoginValidator] that can
// be used to validate logins by clicking links in emails sent to a SMTP4Dev
// server.
func NewSMTP4DevLoginValidator(endpoint string, options ...SMTP4DevLoginValidatorOption) (*SMTP4DevLoginValidator, error) {
	cfg := clickerConfig{httpClient: http.DefaultClient}
	for _, option := range options {
		option(&cfg)
	}
	if cfg.clicker == nil {
		cfg.clicker = cfg.httpClient
	}
	client, err := smtp4dev.New(endpoint, smtp4dev.WithHTTPClient(cfg.httpClient))
	if err != nil {
		return nil, err
	}
	return &SMTP4DevLoginValidator{
		Client:     client,
		HTTPClient: cfg.httpClient,
		Clicker:    cfg.clicker,
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
				res, err := ec.Clicker.Do(req)
				if err != nil {
					return fmt.Errorf("clicking validation link: %w", err)
				}
				defer res.Body.Close()
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
