package smtp4dev

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/google/uuid"
)

// HTTPDoer issues HTTP requests. It matches [http.Client.Do] so an
// *http.Client satisfies it, and custom transports (eg. one that runs
// curl inside a container) can too.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

type messagesConfig struct {
	page     int
	pageSize int
}

type MessagesOption func(*messagesConfig)

func WithPage(page int) MessagesOption {
	return func(c *messagesConfig) {
		c.page = page
	}
}

func WithPageSize(pageSize int) MessagesOption {
	return func(c *messagesConfig) {
		c.pageSize = pageSize
	}
}

type Option func(*Client)

// WithDoer configures the client to use a custom HTTP doer. Defaults to
// [http.DefaultClient].
func WithDoer(doer HTTPDoer) Option {
	return func(c *Client) {
		c.doer = doer
	}
}

type Client struct {
	endpoint url.URL
	doer     HTTPDoer
}

// New creates a new SMTP4Dev client for the given API endpoint.
// See https://github.com/rnwood/smtp4dev/blob/master/docs/Testing.md#api-endpoints
func New(endpoint string, options ...Option) (*Client, error) {
	parsedURL, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	client := &Client{endpoint: *parsedURL, doer: http.DefaultClient}
	for _, option := range options {
		option(client)
	}
	return client, nil
}

// Messages returns a page of all messages sent to the SMTP server.
func (c *Client) Messages(ctx context.Context, options ...MessagesOption) (MessagePage, error) {
	config := &messagesConfig{page: 0, pageSize: 100}
	for _, option := range options {
		option(config)
	}

	url := c.endpoint.JoinPath("api", "messages")
	query := url.Query()
	query.Set("page", strconv.Itoa(config.page))
	query.Set("pageSize", strconv.Itoa(config.pageSize))
	url.RawQuery = query.Encode()

	return jsonRequest[MessagePage](ctx, c.doer, http.MethodGet, url.String(), nil)
}

func (c *Client) Message(ctx context.Context, id uuid.UUID) (Message, error) {
	url := c.endpoint.JoinPath("api", "messages", id.String())
	return jsonRequest[Message](ctx, c.doer, http.MethodGet, url.String(), nil)
}

func (c *Client) DeleteMessage(ctx context.Context, id uuid.UUID) error {
	url := c.endpoint.JoinPath("api", "messages", id.String())
	_, err := request(ctx, c.doer, http.MethodDelete, url.String(), nil)
	return err
}

// MessageBodyPlainText returns the plain text body of the message with the
// given ID if there is one.
func (c *Client) MessageBodyPlainText(ctx context.Context, id uuid.UUID) (string, error) {
	url := c.endpoint.JoinPath("api", "messages", id.String(), "plaintext")
	body, err := request(ctx, c.doer, http.MethodGet, url.String(), nil)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func request(ctx context.Context, doer HTTPDoer, method string, url string, body io.Reader) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	res, err := doer.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected status code: %d", res.StatusCode)
	}
	buf, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}
	return buf, nil
}

func jsonRequest[T any](ctx context.Context, doer HTTPDoer, method string, url string, body io.Reader) (T, error) {
	var zero T
	resBody, err := request(ctx, doer, method, url, body)
	if err != nil {
		return zero, err
	}
	if err := json.Unmarshal(resBody, &zero); err != nil {
		return zero, fmt.Errorf("decoding response: %w", err)
	}
	return zero, nil
}
