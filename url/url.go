package url

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// ParseAndValidate parses a URL string and validates it has a scheme and host.
func ParseAndValidate(rawURL string) (*url.URL, error) {
	if strings.TrimSpace(rawURL) == "" {
		return nil, fmt.Errorf("url cannot be empty")
	}

	parsedURL, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}

	if parsedURL.Scheme == "" || parsedURL.Host == "" {
		return nil, fmt.Errorf("url must be absolute with scheme (http/https) and host")
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return nil, fmt.Errorf("url scheme must be http or https")
	}

	return parsedURL, nil
}

// ValidateExternal validates that a URL is external and not pointing to private/internal IP addresses.
func ValidateExternal(rawURL string) (*url.URL, error) {
	parsedURL, err := ParseAndValidate(rawURL)
	if err != nil {
		return nil, err
	}

	if err := ValidateNotPrivate(parsedURL.Host); err != nil {
		return nil, err
	}

	return parsedURL, nil
}

// ValidateNotPrivate checks if a host (hostname or hostname:port) resolves to a private or loopback IP address.
func ValidateNotPrivate(host string) error {
	hostname, _, err := net.SplitHostPort(host)
	if err != nil {
		hostname = host
	}

	hostname = strings.Trim(hostname, "[]")

	if ip := net.ParseIP(hostname); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() {
			return fmt.Errorf("requests to private IP addresses are not allowed: %s", hostname)
		}
		return nil
	}

	ips, err := net.LookupIP(hostname)
	if err != nil {
		return nil
	}

	for _, resolvedIP := range ips {
		if resolvedIP.IsLoopback() || resolvedIP.IsPrivate() {
			return fmt.Errorf("url resolves to private IP address: %s -> %s", hostname, resolvedIP.String())
		}
	}

	return nil
}

// ExtractHost extracts the host (hostname:port or just hostname) from a URL string.
func ExtractHost(urlStr string) (string, error) {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return "", fmt.Errorf("invalid url: %w", err)
	}

	if parsedURL.Host == "" {
		return "", fmt.Errorf("url has no host: %s", urlStr)
	}

	return parsedURL.Host, nil
}
