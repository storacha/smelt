package guppy

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"

	"github.com/storacha/smelt/pkg/stack"
)

// ExecDoer is an http.Client-shaped HTTP doer that issues requests via `curl`
// inside a container on the stack's Docker network. It exists so that code
// running on the host can POST to URLs that only resolve in-network — notably
// the validation links sprue embeds in its emails, which use the `upload`
// Docker DNS name instead of a host port (see pkg/stack/ports.go).
//
// Request bodies are not supported: stack.Exec can't pipe stdin into the
// container, and the only caller (login validation) sends no body.
type ExecDoer struct {
	Stack   *stack.Stack
	Service string
}

// Do implements http.Client.Do for a narrow subset of requests: method,
// headers, URL, no body.
func (d *ExecDoer) Do(req *http.Request) (*http.Response, error) {
	if req.Body != nil && req.Body != http.NoBody {
		return nil, fmt.Errorf("ExecDoer does not support request bodies")
	}

	ctx := req.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	args := []string{"curl", "-sS", "-i", "-X", req.Method}
	for name, values := range req.Header {
		for _, v := range values {
			args = append(args, "-H", fmt.Sprintf("%s: %s", name, v))
		}
	}
	args = append(args, req.URL.String())

	stdout, stderr, err := d.Stack.Exec(ctx, d.Service, args...)
	if err != nil {
		return nil, fmt.Errorf("curl in %s: %w (stderr: %s)", d.Service, err, strings.TrimSpace(stderr))
	}
	return parseCurlResponse(stdout, req)
}

// parseCurlResponse parses the `curl -sS -i` dump of an HTTP response into a
// real *http.Response. We can't hand the raw output to http.ReadResponse
// because curl decodes chunked transfer-encoding on its side while leaving the
// `Transfer-Encoding: chunked` header in place; ReadResponse then tries to
// decode chunk framing that isn't there. Parsing status + headers by hand and
// treating the rest as a decoded body sidesteps that.
func parseCurlResponse(output string, req *http.Request) (*http.Response, error) {
	br := bufio.NewReader(strings.NewReader(output))

	statusLine, err := br.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("reading status line: %w", err)
	}
	statusLine = strings.TrimRight(statusLine, "\r\n")
	parts := strings.SplitN(statusLine, " ", 3)
	if len(parts) < 2 {
		return nil, fmt.Errorf("malformed status line: %q", statusLine)
	}
	statusCode, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil, fmt.Errorf("parsing status code: %w", err)
	}

	tp := textproto.NewReader(br)
	mimeHeader, err := tp.ReadMIMEHeader()
	if err != nil {
		return nil, fmt.Errorf("reading headers: %w", err)
	}

	body, err := io.ReadAll(br)
	if err != nil {
		return nil, fmt.Errorf("reading body: %w", err)
	}

	return &http.Response{
		Status:        fmt.Sprintf("%d %s", statusCode, http.StatusText(statusCode)),
		StatusCode:    statusCode,
		Proto:         parts[0],
		Header:        http.Header(mimeHeader),
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       req,
	}, nil
}
