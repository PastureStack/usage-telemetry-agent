package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

const maxPublishedRecordBytes = 1 << 20

type Publisher struct {
	target    string
	client    *http.Client
	userAgent string
}

func NewPublisher(target string, timeout time.Duration, version string) (*Publisher, error) {
	if err := validateTargetURL(target); err != nil {
		return nil, err
	}
	transport := newHTTPTransport(true)
	return &Publisher{
		target: target,
		client: &http.Client{
			Timeout:   timeout,
			Transport: transport,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		userAgent: "PastureStack-Usage-Telemetry-Agent/" + version,
	}, nil
}

func (p *Publisher) Report(ctx context.Context, record map[string]any) error {
	payload, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if len(payload) > maxPublishedRecordBytes {
		return errors.New("aggregate record exceeded 1 MiB")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.target, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", p.userAgent)
	response, err := p.client.Do(request)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return errors.New("publishing request failed")
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("publishing target returned HTTP %d", response.StatusCode)
	}
	return nil
}
