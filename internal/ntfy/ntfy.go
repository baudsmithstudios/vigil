package ntfy

import (
	"fmt"
	"net/url"
	"strings"
)

const DefaultServer = "https://ntfy.sh"

func ValidateServerURL(raw string) error {
	u, err := parseHTTPSURL(raw, "server URL")
	if err != nil {
		return err
	}
	if u.User != nil {
		return fmt.Errorf("server URL must not include credentials")
	}
	if u.Path != "" && u.Path != "/" {
		return fmt.Errorf("server URL must not include a path")
	}
	if u.RawQuery != "" {
		return fmt.Errorf("server URL must not include a query")
	}
	if u.Fragment != "" {
		return fmt.Errorf("server URL must not include a fragment")
	}
	return nil
}

func ValidateTopic(topic string) error {
	if topic == "" {
		return fmt.Errorf("topic must not be empty")
	}
	for _, r := range topic {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			continue
		}
		return fmt.Errorf("topic must contain only letters, numbers, hyphens, and underscores")
	}
	return nil
}

func parseHTTPSURL(raw, label string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid %s: %w", label, err)
	}
	if !strings.EqualFold(u.Scheme, "https") {
		return nil, fmt.Errorf("%s must use HTTPS (got %s)", label, u.Scheme)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("%s missing host", label)
	}
	return u, nil
}
