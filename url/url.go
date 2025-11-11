package url

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// Normalize normalizes a URL for deduplication purposes.
func Normalize(rawURL string) (string, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse URL: %w", err)
	}

	if parsedURL.Scheme == "" || parsedURL.Host == "" {
		return "", fmt.Errorf("not a valid absolute URL")
	}

	parsedURL.Fragment = ""

	hostname := parsedURL.Hostname()
	port := parsedURL.Port()

	if after, ok := strings.CutPrefix(hostname, "www."); ok {
		hostname = after
	}

	if parsedURL.Scheme == "http" {
		if port == "80" {
			port = ""
		}
		parsedURL.Scheme = "https"
	}

	if parsedURL.Scheme == "https" && port == "443" {
		port = ""
	}

	if port != "" {
		parsedURL.Host = hostname + ":" + port
	} else {
		parsedURL.Host = hostname
	}

	path := parsedURL.Path

	indexFiles := []string{"/index.html", "/index.htm", "/index.php", "/index.shtml", "/index.xml"}
	for _, indexFile := range indexFiles {
		if strings.HasSuffix(path, indexFile) {
			path = strings.TrimSuffix(path, indexFile)
			if path == "" {
				path = "/"
			}
			break
		}
	}

	if len(path) > 1 && strings.HasSuffix(path, "/") {
		path = strings.TrimSuffix(path, "/")
	}

	if path == "" {
		path = "/"
	}

	parsedURL.Path = path

	return parsedURL.String(), nil
}

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
// Returns the parsed URL if validation succeeds.
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

// IsSameBaseDomain checks if two URLs belong to the same base/root domain.
func IsSameBaseDomain(url1, url2 string) bool {
	parsed1, err1 := url.Parse(url1)
	parsed2, err2 := url.Parse(url2)

	if err1 != nil || err2 != nil {
		return false
	}

	base1 := extractBaseDomain(parsed1.Hostname())
	base2 := extractBaseDomain(parsed2.Hostname())

	return base1 != "" && base2 != "" && base1 == base2
}

// extractBaseDomain extracts the base/root domain from a hostname.
func extractBaseDomain(hostname string) string {
	if hostname == "" {
		return ""
	}

	if net.ParseIP(hostname) != nil {
		return hostname
	}

	if hostname == "localhost" {
		return hostname
	}

	parts := strings.Split(hostname, ".")

	if len(parts) < 2 {
		return hostname
	}

	baseDomain := parts[len(parts)-2] + "." + parts[len(parts)-1]

	if len(parts) >= 3 {
		tld := parts[len(parts)-1]
		sld := parts[len(parts)-2]

		multiPartTLDs := map[string]bool{
			"co":  true,
			"com": true,
			"gov": true,
			"ac":  true,
			"org": true,
			"net": true,
		}

		if multiPartTLDs[sld] {
			baseDomain = parts[len(parts)-3] + "." + sld + "." + tld
		}
	}

	return baseDomain
}
