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

// ExecDoer issues HTTP requests by invoking curl inside a container on the
// Docker network. Useful for reaching services that aren't published to the
// host, eg. during stack tests.
//
// Only nil request bodies are supported today: stack.Exec has no way to
// pipe stdin into the container, so sending bodies would require extra
// machinery (tempfile + separate exec). The one caller (login validation)
// never sends a body.
type ExecDoer struct {
	Stack   *stack.Stack
	Service string
}

// Do implements [smtp4dev.HTTPDoer] / the subset of *http.Client used here.
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

// parseCurlResponse parses the output of `curl -sS -i` into an http.Response.
// curl decodes chunked transfer-encoding on its side but leaves the header
// in place, so we can't feed the raw output to http.ReadResponse — it would
// try to decode chunk framing that isn't there. Instead, parse the status
// line and headers ourselves, then treat the rest as the body.
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
