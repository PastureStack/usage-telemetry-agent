package agent

import (
	"bytes"
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestVersionOutputUsesNeutralExecutableName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Main([]string{"usage-telemetry-agent", "--version"}, "0.4.1", "abc123", &stdout, &stderr)
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("unexpected result code=%d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "usage-telemetry-agent 0.4.1 (abc123)") {
		t.Fatalf("unexpected version output: %q", stdout.String())
	}
}

func TestHandlerRejectsBrowserOriginPOST(t *testing.T) {
	svc := &service{
		root:    context.Background(),
		timeout: time.Second,
		logger:  log.New(io.Discard, "", 0),
	}
	request := httptest.NewRequest(http.MethodPost, "/v1-telemetry/report", nil)
	request.Header.Set("Origin", "https://browser.example.invalid")
	recorder := httptest.NewRecorder()
	svc.handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), "browser-origin") {
		t.Fatalf("unexpected browser-origin response: code=%d body=%q", recorder.Code, recorder.Body.String())
	}
}
