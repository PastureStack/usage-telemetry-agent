package agent

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPublisherPostsOnlyToExplicitTarget(t *testing.T) {
	received := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.Header.Get("Content-Type") != "application/json" {
			http.Error(writer, "bad request", http.StatusBadRequest)
			return
		}
		var record map[string]any
		if err := json.NewDecoder(request.Body).Decode(&record); err != nil {
			http.Error(writer, "bad json", http.StatusBadRequest)
			return
		}
		received <- record
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	publisher, err := NewPublisher(server.URL, time.Second, "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := publisher.Report(context.Background(), map[string]any{"r": 1}); err != nil {
		t.Fatal(err)
	}
	select {
	case record := <-received:
		if record["r"].(float64) != 1 {
			t.Fatalf("unexpected record: %#v", record)
		}
	case <-time.After(time.Second):
		t.Fatal("publisher did not send the aggregate")
	}
}

func TestPublisherRefusesRedirect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, "https://outside.example.invalid/collect", http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	publisher, err := NewPublisher(server.URL, time.Second, "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := publisher.Report(context.Background(), map[string]any{"r": 1}); err == nil {
		t.Fatal("expected redirect rejection")
	}
}

func TestPublisherRequiresModernTLSAndHidesFailedTarget(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	target := server.URL
	server.Close()
	publisher, err := NewPublisher(target, 50*time.Millisecond, "test")
	if err != nil {
		t.Fatal(err)
	}
	transport := publisher.client.Transport.(*http.Transport)
	if transport.TLSClientConfig == nil || transport.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Fatal("publisher did not require TLS 1.2 or newer")
	}
	err = publisher.Report(context.Background(), map[string]any{"r": 1})
	if err == nil || err.Error() != "publishing request failed" {
		t.Fatalf("unexpected sanitized error: %v", err)
	}
}
