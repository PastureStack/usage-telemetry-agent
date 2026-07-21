package agent

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestAPIClientUsesBasicAuthAndRejectsCrossOriginPagination(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		user, password, ok := request.BasicAuth()
		if !ok || user != "access" || password != "secret" {
			http.Error(writer, "unauthorized", http.StatusUnauthorized)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprint(writer, `{"data":[],"pagination":{"next":"https://outside.example.invalid/v2-beta/hosts"}}`)
	}))
	defer server.Close()
	client, err := NewAPIClient(server.URL+"/v2-beta", "access", "secret", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.List(context.Background(), "hosts", url.Values{}); err == nil {
		t.Fatal("expected cross-origin pagination rejection")
	}
}

func TestAPIClientRejectsInvalidResource(t *testing.T) {
	client, err := NewAPIClient("http://127.0.0.1:8081/v2-beta", "access", "secret", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.List(context.Background(), "../settings", nil); err == nil {
		t.Fatal("expected invalid resource rejection")
	}
}

func TestAPIClientDisablesAmbientProxyAndRequiresModernTLS(t *testing.T) {
	client, err := NewAPIClient("http://127.0.0.1:8081/v2-beta", "access", "secret", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	transport := client.http.Transport.(*http.Transport)
	if transport.Proxy != nil {
		t.Fatal("credentialed API transport unexpectedly uses an ambient proxy")
	}
	if transport.TLSClientConfig == nil || transport.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Fatal("credentialed API transport did not require TLS 1.2 or newer")
	}
}

func TestAPIClientHidesFailedRequestURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	endpoint := server.URL
	server.Close()
	client, err := NewAPIClient(endpoint, "access", "secret", 50*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.List(context.Background(), "hosts", nil)
	if err == nil || err.Error() != "API request failed" {
		t.Fatalf("unexpected sanitized error: %v", err)
	}
}

func TestAPIClientReadsSettingByID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v2-beta/settings/server.version" || request.URL.RawQuery != "" {
			http.Error(writer, "unexpected path", http.StatusBadRequest)
			return
		}
		fmt.Fprint(writer, `{"name":"server.version","value":"v1.6.270"}`)
	}))
	defer server.Close()
	client, err := NewAPIClient(server.URL+"/v2-beta", "access", "secret", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	value, err := client.Setting(context.Background(), "server.version")
	if err != nil || value != "v1.6.270" {
		t.Fatalf("unexpected setting value=%q err=%v", value, err)
	}
}
