package agent

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxAPIResponseBytes = 8 << 20

type APIClient struct {
	base      *url.URL
	accessKey string
	secretKey string
	http      *http.Client
}

type collectionResponse struct {
	Data       []map[string]any `json:"data"`
	Pagination *struct {
		Next string `json:"next"`
	} `json:"pagination"`
}

func validPathSegment(value string) bool {
	return value != "" && !strings.ContainsAny(value, "/\\?#")
}

func NewAPIClient(rawURL, accessKey, secretKey string, timeout time.Duration) (*APIClient, error) {
	if err := validateAPIURL(rawURL); err != nil {
		return nil, err
	}
	base, _ := url.Parse(strings.TrimRight(rawURL, "/"))
	transport := newHTTPTransport(false)
	return &APIClient{
		base:      base,
		accessKey: accessKey,
		secretKey: secretKey,
		http: &http.Client{
			Timeout:   timeout,
			Transport: transport,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

func newHTTPTransport(useProxy bool) *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if !useProxy {
		// Control-plane credentials must never be forwarded through an ambient
		// HTTP proxy. The compatible launcher always supplies a local API URL.
		transport.Proxy = nil
	}
	if transport.TLSClientConfig == nil {
		transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	} else {
		transport.TLSClientConfig = transport.TLSClientConfig.Clone()
		transport.TLSClientConfig.MinVersion = tls.VersionTLS12
	}
	return transport
}

func (c *APIClient) List(ctx context.Context, resource string, filters url.Values) ([]map[string]any, error) {
	if !validPathSegment(resource) {
		return nil, errors.New("invalid API resource")
	}
	endpoint := *c.base
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/" + resource
	query := endpoint.Query()
	query.Set("limit", "1000")
	for key, values := range filters {
		for _, value := range values {
			query.Add(key, value)
		}
	}
	endpoint.RawQuery = query.Encode()

	var result []map[string]any
	for page := 0; page < 100; page++ {
		response, err := c.getPage(ctx, &endpoint)
		if err != nil {
			return nil, err
		}
		result = append(result, response.Data...)
		if response.Pagination == nil || strings.TrimSpace(response.Pagination.Next) == "" {
			return result, nil
		}
		next, err := endpoint.Parse(response.Pagination.Next)
		if err != nil || !sameOrigin(c.base, next) {
			return nil, errors.New("API pagination attempted to leave the configured origin")
		}
		endpoint = *next
	}
	return nil, errors.New("API pagination exceeded 100 pages")
}

func (c *APIClient) getPage(ctx context.Context, endpoint *url.URL) (collectionResponse, error) {
	var decoded collectionResponse
	if err := c.getJSON(ctx, endpoint, &decoded); err != nil {
		return collectionResponse{}, err
	}
	return decoded, nil
}

func (c *APIClient) getObject(ctx context.Context, resource, id string) (map[string]any, error) {
	if !validPathSegment(resource) || !validPathSegment(id) {
		return nil, errors.New("invalid API object path")
	}
	endpoint := *c.base
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/" + resource + "/" + id
	var decoded map[string]any
	if err := c.getJSON(ctx, &endpoint, &decoded); err != nil {
		return nil, err
	}
	return decoded, nil
}

func (c *APIClient) getJSON(ctx context.Context, endpoint *url.URL, destination any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "PastureStack-Usage-Telemetry-Agent")
	request.SetBasicAuth(c.accessKey, c.secretKey)
	response, err := c.http.Do(request)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return errors.New("API request failed")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return fmt.Errorf("API returned HTTP %d", response.StatusCode)
	}
	limited := io.LimitReader(response.Body, maxAPIResponseBytes+1)
	payload, err := io.ReadAll(limited)
	if err != nil {
		return err
	}
	if len(payload) > maxAPIResponseBytes {
		return errors.New("API response exceeded 8 MiB")
	}
	if err := json.Unmarshal(payload, destination); err != nil {
		return errors.New("API returned invalid JSON")
	}
	return nil
}

func (c *APIClient) Setting(ctx context.Context, name string) (string, error) {
	item, err := c.getObject(ctx, "settings", name)
	if err != nil {
		return "", err
	}
	return stringValue(item["value"]), nil
}

func sameOrigin(base, candidate *url.URL) bool {
	return candidate.User == nil && candidate.Fragment == "" &&
		strings.EqualFold(base.Scheme, candidate.Scheme) && strings.EqualFold(base.Host, candidate.Host)
}
