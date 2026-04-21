package guppy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseCurlResponse(t *testing.T) {
	// Shape mirrors `curl -sS -i` output: status line, CRLF headers, blank line, body.
	raw := "HTTP/1.1 204 No Content\r\n" +
		"Server: test\r\n" +
		"Content-Length: 0\r\n" +
		"\r\n"

	req := httptest.NewRequest(http.MethodPost, "http://upload/validate-email/x", nil)
	resp, err := parseCurlResponse(raw, req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 204 {
		t.Errorf("want 204, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Server"); got != "test" {
		t.Errorf("want Server=test, got %q", got)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) != 0 {
		t.Errorf("want empty body, got %q", body)
	}
}

func TestParseCurlResponseWithBody(t *testing.T) {
	raw := "HTTP/1.1 200 OK\r\n" +
		"Content-Type: text/plain\r\n" +
		"\r\n" +
		"hello world"

	req := httptest.NewRequest(http.MethodGet, "http://upload/hi", nil)
	resp, err := parseCurlResponse(raw, req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "hello world" {
		t.Errorf("want %q, got %q", "hello world", string(body))
	}
}
