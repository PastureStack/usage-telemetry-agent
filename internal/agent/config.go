package agent

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultListen   = "127.0.0.1:8114"
	defaultInterval = 6 * time.Hour
	defaultTimeout  = 15 * time.Second
)

type Config struct {
	APIURL                string
	AccessKey             string
	SecretKey             string
	Listen                string
	Interval              time.Duration
	RequestTimeout        time.Duration
	TargetURL             string
	Once                  bool
	IncludeInstallationID bool
	AllowRemoteListen     bool
}

type envLookup func(string) (string, bool)

func parseConfig(args []string, lookup envLookup, stderr io.Writer) (Config, error) {
	value := func(primary string, compatibility ...string) string {
		if raw, ok := lookup(primary); ok && strings.TrimSpace(raw) != "" {
			return strings.TrimSpace(raw)
		}
		for _, name := range compatibility {
			if raw, ok := lookup(name); ok && strings.TrimSpace(raw) != "" {
				return strings.TrimSpace(raw)
			}
		}
		return ""
	}

	interval := defaultInterval
	if raw := value("PASTURESTACK_USAGE_TELEMETRY_INTERVAL", "TELEMETRY_INTERVAL"); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, fmt.Errorf("invalid reporting interval: %w", err)
		}
		interval = parsed
	}
	timeout := defaultTimeout
	if raw := value("PASTURESTACK_USAGE_TELEMETRY_REQUEST_TIMEOUT"); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, fmt.Errorf("invalid request timeout: %w", err)
		}
		timeout = parsed
	}

	cfg := Config{
		APIURL:         value("PASTURESTACK_API_URL", "CATTLE_URL"),
		AccessKey:      value("PASTURESTACK_API_ACCESS_KEY", "CATTLE_ACCESS_KEY"),
		SecretKey:      value("PASTURESTACK_API_SECRET_KEY", "CATTLE_SECRET_KEY"),
		Listen:         value("PASTURESTACK_USAGE_TELEMETRY_LISTEN", "TELEMETRY_LISTEN"),
		Interval:       interval,
		RequestTimeout: timeout,
		TargetURL:      value("PASTURESTACK_USAGE_TELEMETRY_TARGET_URL"),
	}
	if cfg.Listen == "" {
		cfg.Listen = defaultListen
	}
	if raw := value("PASTURESTACK_USAGE_TELEMETRY_INCLUDE_INSTALLATION_ID"); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("invalid installation ID option: %w", err)
		}
		cfg.IncludeInstallationID = parsed
	}
	if raw := value("PASTURESTACK_USAGE_TELEMETRY_ALLOW_REMOTE_LISTEN"); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("invalid remote-listen option: %w", err)
		}
		cfg.AllowRemoteListen = parsed
	}

	set := flag.NewFlagSet("client", flag.ContinueOnError)
	set.SetOutput(stderr)
	set.BoolVar(&cfg.Once, "once", false, "print one aggregate record and exit without publishing")
	set.StringVar(&cfg.Listen, "listen", cfg.Listen, "local HTTP listen address")
	set.StringVar(&cfg.APIURL, "url", cfg.APIURL, "compatible control-plane API URL")
	set.StringVar(&cfg.AccessKey, "access-key", cfg.AccessKey, "compatible API access key")
	set.StringVar(&cfg.SecretKey, "secret-key", cfg.SecretKey, "compatible API secret key")
	set.DurationVar(&cfg.Interval, "interval", cfg.Interval, "reporting interval when an explicit target is configured")
	set.DurationVar(&cfg.RequestTimeout, "request-timeout", cfg.RequestTimeout, "API and publisher request timeout")
	set.StringVar(&cfg.TargetURL, "target-url", cfg.TargetURL, "explicit HTTPS publishing target; empty disables publishing")
	set.BoolVar(&cfg.IncludeInstallationID, "include-installation-id", cfg.IncludeInstallationID, "include a one-way hash of an existing installation identifier")
	set.BoolVar(&cfg.AllowRemoteListen, "allow-remote-listen", cfg.AllowRemoteListen, "allow binding the local aggregate endpoint beyond loopback")
	if err := set.Parse(args); err != nil {
		return Config{}, err
	}
	if set.NArg() != 0 {
		return Config{}, fmt.Errorf("unexpected arguments: %s", strings.Join(set.Args(), " "))
	}
	if strings.TrimSpace(cfg.APIURL) == "" || strings.TrimSpace(cfg.AccessKey) == "" || strings.TrimSpace(cfg.SecretKey) == "" {
		return Config{}, errors.New("API URL, access key, and secret key are required")
	}
	if err := validateAPIURL(cfg.APIURL); err != nil {
		return Config{}, err
	}
	if cfg.Interval < time.Minute {
		return Config{}, errors.New("reporting interval must be at least one minute")
	}
	if cfg.RequestTimeout <= 0 || cfg.RequestTimeout > time.Minute {
		return Config{}, errors.New("request timeout must be greater than zero and no more than one minute")
	}
	if cfg.TargetURL != "" {
		if err := validateTargetURL(cfg.TargetURL); err != nil {
			return Config{}, err
		}
	}
	if err := validateListen(cfg.Listen, cfg.AllowRemoteListen); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func validateAPIURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return errors.New("API URL must be an absolute HTTP or HTTPS URL")
	}
	if parsed.User != nil || parsed.Fragment != "" || parsed.RawQuery != "" {
		return errors.New("API URL must not contain user information, a query, or a fragment")
	}
	if parsed.Scheme == "http" && !isLoopbackHost(parsed.Hostname()) {
		return errors.New("API URL must use HTTPS; HTTP is allowed only for the local control-plane API")
	}
	return nil
}

func validateTargetURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" {
		return errors.New("publishing target must be an absolute URL")
	}
	if parsed.User != nil || parsed.Fragment != "" {
		return errors.New("publishing target must not contain user information or a fragment")
	}
	if parsed.Scheme == "https" {
		return nil
	}
	if parsed.Scheme == "http" && isLoopbackHost(parsed.Hostname()) {
		return nil
	}
	return errors.New("publishing target must use HTTPS; HTTP is allowed only for loopback testing")
}

func validateListen(address string, allowRemote bool) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil || port == "" {
		return errors.New("listen address must use host:port form")
	}
	if allowRemote {
		return nil
	}
	if host == "localhost" || isLoopbackHost(host) {
		return nil
	}
	return errors.New("listen address must be loopback unless --allow-remote-listen is explicitly set")
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}
