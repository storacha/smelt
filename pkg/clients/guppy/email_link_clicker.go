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

type clickerConifg struct {
	httpClient *http.Client
}

type Option func(*clickerConifg)

func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *clickerConifg) {
		c.httpClient = httpClient
	}
}

type EmailClicker struct {
	apiClient  *smtp4dev.Client
	httpClient *http.Client
}

// NewSMTP4DevEmailLinkClicker returns a new EmailClicker that can be used to
// click links in emails sent to a SMTP4Dev server.
func NewSMTP4DevEmailLinkClicker(endpoint string, options ...Option) (*EmailClicker, error) {
	cfg := clickerConifg{httpClient: http.DefaultClient}
	for _, option := range options {
		option(&cfg)
	}
	client, err := smtp4dev.New(endpoint, smtp4dev.WithHTTPClient(cfg.httpClient))
	if err != nil {
		return nil, err
	}
	return &EmailClicker{
		apiClient:  client,
		httpClient: cfg.httpClient,
	}, nil
}

// ClickValidationLink polls the SMTP4Dev server for an email sent to the given
// address, extracts the validation link, and clicks it. Use the passed context
// to cancel the polling.
func (ec *EmailClicker) ClickValidationLink(ctx context.Context, email string) error {
	found := false
	for !found {
		select {
		case <-time.After(100 * time.Millisecond):
		case <-ctx.Done():
			return ctx.Err()
		}

		page := 0
		for !found {
			msgPage, err := ec.apiClient.Messages(ctx, smtp4dev.WithPage(page))
			if err != nil {
				return fmt.Errorf("fetching messages: %w", err)
			}
			for _, msg := range msgPage.Results {
				if msg.DeliveredTo != email {
					continue
				}
				body, err := ec.apiClient.MessageBodyPlainText(ctx, msg.ID)
				if err != nil {
					return fmt.Errorf("fetching message body: %w", err)
				}
				link, err := extractValidationLink(body)
				if err != nil {
					continue
				}
				res, err := ec.httpClient.Post(link.String(), "text/plain", nil)
				if err != nil {
					return fmt.Errorf("clicking validation link: %w", err)
				}
				defer res.Body.Close()
				if res.StatusCode < 200 || res.StatusCode >= 300 {
					return fmt.Errorf("clicking validation link: received status code %d", res.StatusCode)
				}
				// clean up the message
				err = ec.apiClient.DeleteMessage(ctx, msg.ID)
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
	if strings.Contains(body, "validate-email") {
		return url.URL{}, fmt.Errorf("validation link not found")
	}
	u, err := url.Parse(strings.TrimSpace(body))
	if err != nil {
		return url.URL{}, fmt.Errorf("parsing validation link: %w", err)
	}
	return *u, nil
}
