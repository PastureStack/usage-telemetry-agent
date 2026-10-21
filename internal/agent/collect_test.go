package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCollectorProducesAggregatesWithoutPrivateIdentifiers(t *testing.T) {
	server := fixtureAPIServer(t)
	defer server.Close()
	client, err := NewAPIClient(server.URL+"/v2-beta", "fixture-access", "fixture-secret", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	collector := NewCollector(client, false)
	collector.now = func() time.Time { return time.Date(2026, 7, 22, 1, 2, 3, 0, time.UTC) }
	record, err := collector.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	text := string(payload)
	for _, forbidden := range []string{
		"private-user", "private-host", "private.example", "192.0.2.44",
		"fixture-access", "fixture-secret", "installation-uuid", "catalog-private-name", "build private-user",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("aggregate leaked %q: %s", forbidden, text)
		}
	}
	install := record["install"].(installationStats)
	if install.UID != "not-collected" || install.Image != "custom" || install.Version != "v1.6.270" {
		t.Fatalf("unexpected installation summary: %#v", install)
	}
	containers := record["container"].(containerStats)
	if containers.Total != 3 || containers.Running != 2 || containers.PerHostMax != 2 {
		t.Fatalf("unexpected container summary: %#v", containers)
	}
	services := record["service"].(serviceStats)
	if services.Total != 2 || services.Active != 1 || services.Kind["service"] != 2 {
		t.Fatalf("unexpected service summary: %#v", services)
	}
	hosts := record["host"].(hostStats)
	if hosts.Kernel["6.8"] != 1 || hosts.OS["ubuntu"] != 1 || hosts.Docker["v29.4.2"] != 1 {
		t.Fatalf("host labels were not reduced to safe categories: %#v", hosts)
	}
}

func TestCategoricalValuesDoNotPassThroughArbitraryLabels(t *testing.T) {
	checks := []struct {
		actual   string
		expected string
	}{
		{normalizeOrchestration("private-orchestrator"), "other"},
		{normalizeAuthProvider("private-provider"), "other"},
		{normalizeMachineDriver("private-driver"), "other"},
		{normalizeServiceKind("private-service-kind"), "other"},
		{classifyOperatingSystem("PrivateOS private-host 1.0"), "other"},
		{normalizeKernelVersion("private-kernel"), "other"},
		{normalizeDockerVersion("private-engine private-user"), "other"},
		{safeVersion("1.6.270-private-user"), "custom"},
	}
	for _, check := range checks {
		if check.actual != check.expected {
			t.Fatalf("unexpected normalized category %q; want %q", check.actual, check.expected)
		}
	}
}

func TestCollectorHashesExistingInstallationIDOnlyWhenExplicit(t *testing.T) {
	server := fixtureAPIServer(t)
	defer server.Close()
	client, err := NewAPIClient(server.URL+"/v2-beta", "fixture-access", "fixture-secret", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	record, err := NewCollector(client, true).Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	install := record["install"].(installationStats)
	if !strings.HasPrefix(install.UID, "sha256:") || strings.Contains(install.UID, "installation-uuid") {
		t.Fatalf("installation ID was not safely transformed: %q", install.UID)
	}
}

func fixtureAPIServer(t *testing.T) *httptest.Server {
	t.Helper()
	settings := map[string]string{
		"telemetry.uid":                "installation-uuid-private-user",
		"rancher.server.image":         "ghcr.io/private-user/private-repository:v1.6.270",
		"rancher.server.version":       "v1.6.270",
		"api.security.enabled":         "true",
		"api.auth.provider.configured": "ldapconfig",
	}
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		user, password, ok := request.BasicAuth()
		if !ok || user != "fixture-access" || password != "fixture-secret" {
			http.Error(writer, "unauthorized", http.StatusUnauthorized)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		resource := strings.TrimPrefix(request.URL.Path, "/v2-beta/")
		var data any
		if strings.HasPrefix(resource, "settings/") {
			name := strings.TrimPrefix(resource, "settings/")
			fmt.Fprint(writer, mustJSON(t, map[string]any{"name": name, "value": settings[name]}))
			return
		} else {
			switch resource {
			case "projects":
				data = []map[string]any{{"id": "1a5", "name": "private.example", "orchestration": "native"}}
			case "hosts":
				data = []map[string]any{{
					"id": "1h1", "name": "private-host", "hostname": "private.example", "state": "active",
					"info": map[string]any{
						"cpuInfo":    map[string]any{"count": 2, "mhz": 2400, "cpuCoresPercentages": []any{10, 20}},
						"memoryInfo": map[string]any{"memTotal": 4096, "memAvailable": 3072},
						"osInfo":     map[string]any{"kernelVersion": "6.8.0", "operatingSystem": "Ubuntu 26.04", "dockerVersion": "Docker version 29.4.2, build private-user"},
					},
					"agentIpAddress": "192.0.2.44",
				}}
			case "machines":
				data = []map[string]any{{"driver": "generic", "name": "private-host"}}
			case "containers":
				data = []map[string]any{
					{"state": "running", "hostId": "1h1", "name": "private-a"},
					{"state": "started-once", "hostId": "1h1", "name": "private-b"},
					{"state": "stopped", "hostId": "1h2", "name": "private-c"},
				}
			case "services":
				data = []map[string]any{
					{"state": "active", "stackId": "1st1", "type": "service", "name": "private-a"},
					{"state": "inactive", "stackId": "1st1", "type": "service", "name": "private-b"},
				}
			case "stacks":
				data = []map[string]any{{"state": "active", "accountId": "1a5", "externalId": "catalog://?catalog=catalog-private-name"}}
			default:
				http.NotFound(writer, request)
				return
			}
		}
		fmt.Fprintf(writer, `{"data":%s}`, mustJSON(t, data))
	}))
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(payload)
}
