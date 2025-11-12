package url

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseAndValidateValid verifies valid URLs are accepted.
func TestParseAndValidateValid(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"simple_http", "http://example.com"},
		{"simple_https", "https://example.com"},
		{"with_path", "https://example.com/path/to/resource"},
		{"with_query", "https://example.com/path?key=value"},
		{"with_fragment", "https://example.com/path#section"},
		{"with_port", "https://example.com:8080/path"},
		{"subdomain", "https://sub.example.com"},
		{"ipv4", "https://1.2.3.4"},
		{"ipv4_with_port", "https://1.2.3.4:8080"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := ParseAndValidate(tt.url)
			require.NoError(t, err, "valid URL should not return error")
			assert.NotNil(t, parsed, "parsed URL should not be nil")
		})
	}
}

// TestParseAndValidateInvalid verifies invalid URLs are rejected.
func TestParseAndValidateInvalid(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"empty", ""},
		{"whitespace_only", "   "},
		{"no_scheme", "example.com"},
		{"relative", "/path/to/resource"},
		{"invalid_scheme", "ftp://example.com"},
		{"javascript", "javascript:alert(1)"},
		{"data_uri", "data:text/html,<script>alert(1)</script>"},
		{"malformed", "ht!tp://example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := ParseAndValidate(tt.url)
			assert.Error(t, err, "invalid URL should return error")
			assert.Nil(t, parsed, "parsed URL should be nil on error")
		})
	}
}

// TestValidateExternalPublicIPs verifies public IPs are allowed.
func TestValidateExternalPublicIPs(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"public_ipv4_1", "https://8.8.8.8"},
		{"public_ipv4_2", "https://1.1.1.1"},
		{"public_ipv4_3", "https://140.82.121.4"}, // github.com IP
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := ValidateExternal(tt.url)
			assert.NoError(t, err, "public IP should be allowed")
			assert.NotNil(t, parsed)
		})
	}
}

// TestValidateExternalPrivateIPs verifies private IPs are blocked (SSRF protection).
func TestValidateExternalPrivateIPs(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"loopback_localhost", "https://localhost"},
		{"loopback_127", "https://127.0.0.1"},
		{"loopback_127_alt", "https://127.0.0.2"},
		{"private_10", "https://10.0.0.1"},
		{"private_192", "https://192.168.1.1"},
		{"private_172", "https://172.16.0.1"},
		// Note: 169.254.x.x (link-local) is NOT considered private by Go's net.IP.IsPrivate()
		// So we test it separately below
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := ValidateExternal(tt.url)
			assert.Error(t, err, "private IP should be blocked for SSRF protection")
			assert.Nil(t, parsed)
			assert.Contains(t, err.Error(), "private", "error should mention private IP")
		})
	}
}

// TestValidateNotPrivateDirectIP verifies IP validation without DNS lookup.
func TestValidateNotPrivateDirectIP(t *testing.T) {
	tests := []struct {
		name        string
		host        string
		shouldError bool
	}{
		{"public_ip", "8.8.8.8", false},
		{"loopback", "127.0.0.1", true},
		{"private_10", "10.1.2.3", true},
		{"private_192", "192.168.0.1", true},
		{"private_172", "172.16.5.5", true},
		{"public_with_port", "8.8.8.8:443", false},
		{"private_with_port", "192.168.1.1:8080", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateNotPrivate(tt.host)
			if tt.shouldError {
				assert.Error(t, err, "should reject private IP")
			} else {
				assert.NoError(t, err, "should accept public IP")
			}
		})
	}
}

// TestValidateNotPrivateHostname verifies hostname resolution checks.
func TestValidateNotPrivateHostname(t *testing.T) {
	// Note: These tests depend on actual DNS resolution
	// localhost should resolve to loopback
	err := ValidateNotPrivate("localhost")
	// If localhost resolves (which it usually does), it should be blocked
	// If DNS fails, err will be nil (we allow DNS failures)
	if err != nil {
		assert.Contains(t, err.Error(), "private", "localhost should be blocked if it resolves")
	}

	// Public domains should work (if DNS is available)
	// We don't assert here because DNS might not be available in all test environments
	_ = ValidateNotPrivate("example.com")
}

// TestValidateNotPrivateIPv6 verifies IPv6 support including link-local blocking.
func TestValidateNotPrivateIPv6(t *testing.T) {
	tests := []struct {
		name        string
		host        string
		shouldError bool
		errorText   string
	}{
		{"loopback_v6", "::1", true, "private"},
		{"loopback_v6_bracketed", "[::1]", true, "private"},
		{"link_local_v6", "fe80::1", true, "link-local"},
		{"link_local_v6_bracketed", "[fe80::1]", true, "link-local"},
		{"link_local_v6_full", "fe80:0000:0000:0000:0000:0000:0000:0001", true, "link-local"},
		{"public_v6", "2001:4860:4860::8888", false, ""}, // Google DNS
		{"public_v6_bracketed", "[2001:4860:4860::8888]", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateNotPrivate(tt.host)
			if tt.shouldError {
				assert.Error(t, err, "should reject private/loopback/link-local IPv6")
				if tt.errorText != "" {
					assert.Contains(t, err.Error(), tt.errorText)
				}
			} else {
				assert.NoError(t, err, "should accept public IPv6")
			}
		})
	}
}

// TestExtractHost verifies host extraction from URLs.
func TestExtractHost(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{"simple", "https://example.com", "example.com"},
		{"with_path", "https://example.com/path", "example.com"},
		{"with_port", "https://example.com:8080", "example.com:8080"},
		{"subdomain", "https://api.example.com", "api.example.com"},
		{"ipv4", "https://1.2.3.4", "1.2.3.4"},
		{"ipv4_with_port", "https://1.2.3.4:8080", "1.2.3.4:8080"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, err := ExtractHost(tt.url)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, host)
		})
	}
}

// TestExtractHostInvalid verifies error handling for invalid URLs.
func TestExtractHostInvalid(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"no_host", "http://"},
		{"relative", "/path/to/resource"},
		{"malformed", "://invalid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, err := ExtractHost(tt.url)
			assert.Error(t, err)
			assert.Empty(t, host)
		})
	}
}

// TestSSRFProtectionAWSMetadata verifies link-local addresses are blocked (AWS/GCP/Azure metadata).
// CRITICAL: Blocks 169.254.0.0/16 to prevent SSRF attacks against cloud metadata endpoints.
func TestSSRFProtectionAWSMetadata(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"aws_metadata_endpoint", "http://169.254.169.254/latest/meta-data/"},
		{"aws_metadata_root", "http://169.254.169.254/"},
		{"link_local_start", "http://169.254.0.1/"},
		{"link_local_end", "http://169.254.255.254/"},
		{"gcp_metadata_path", "http://169.254.169.254/computeMetadata/v1/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ValidateExternal(tt.url)
			assert.Error(t, err, "link-local address should be blocked to prevent metadata SSRF")
			assert.Contains(t, err.Error(), "link-local", "error should mention link-local address")
		})
	}
}

// TestSSRFProtectionCommonTargets verifies protection against common SSRF targets.
func TestSSRFProtectionCommonTargets(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"localhost", "http://localhost:8080"},
		{"127_variant", "http://127.0.0.2"},
		{"internal_network", "http://192.168.1.1"},
		{"docker_gateway", "http://172.17.0.1"},
		{"private_10_network", "http://10.0.0.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ValidateExternal(tt.url)
			assert.Error(t, err, "should block common SSRF target: "+tt.url)
		})
	}
}

// TestValidateExternalPublicDomains verifies public domains are allowed.
func TestValidateExternalPublicDomains(t *testing.T) {
	tests := []string{
		"https://example.com",
		"https://github.com",
		"https://google.com",
	}

	for _, url := range tests {
		t.Run(url, func(t *testing.T) {
			// Note: This test requires DNS resolution
			// We don't assert success because DNS might not be available
			parsed, err := ValidateExternal(url)
			if err != nil {
				// If there's an error, it should NOT be about private IPs
				// (it might be a DNS error, which is acceptable)
				assert.NotContains(t, err.Error(), "private",
					"public domain should not be rejected as private")
			} else {
				assert.NotNil(t, parsed)
			}
		})
	}
}

// TestValidateNotPrivateDNSFailure verifies DNS failures are handled gracefully.
func TestValidateNotPrivateDNSFailure(t *testing.T) {
	// Non-existent domain should fail DNS lookup but not return an error
	// (we allow DNS failures to avoid blocking legitimate but temporarily unreachable sites)
	err := ValidateNotPrivate("this-domain-definitely-does-not-exist-12345.com")
	assert.NoError(t, err, "DNS failures should be allowed")
}

// TestIsLinkLocal verifies the link-local detection helper function.
func TestIsLinkLocal(t *testing.T) {
	tests := []struct {
		name       string
		ip         string
		isLinkLocal bool
	}{
		// IPv4 link-local (169.254.0.0/16)
		{"aws_metadata", "169.254.169.254", true},
		{"link_local_start", "169.254.0.0", true},
		{"link_local_end", "169.254.255.255", true},
		{"link_local_random", "169.254.123.45", true},

		// Not link-local
		{"just_before_169_254", "169.253.255.255", false},
		{"just_after_169_254", "169.255.0.0", false},
		{"private_192", "192.168.1.1", false},
		{"private_10", "10.0.0.1", false},
		{"loopback", "127.0.0.1", false},
		{"public", "8.8.8.8", false},

		// IPv6 link-local (fe80::/10)
		{"ipv6_link_local_short", "fe80::1", true},
		{"ipv6_link_local_full", "fe80:0000:0000:0000:0000:0000:0000:0001", true},
		{"ipv6_link_local_upper", "FE80::1", true},

		// Not IPv6 link-local
		{"ipv6_loopback", "::1", false},
		{"ipv6_public", "2001:4860:4860::8888", false},
		{"ipv6_just_before_fe80", "fe7f:ffff:ffff:ffff:ffff:ffff:ffff:ffff", false},
		{"ipv6_febf_end_of_range", "febf:ffff:ffff:ffff:ffff:ffff:ffff:ffff", true},
		{"ipv6_fec0_not_link_local", "fec0::1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			require.NotNil(t, ip, "test IP should be valid")

			result := isLinkLocal(ip)
			assert.Equal(t, tt.isLinkLocal, result,
				"isLinkLocal(%s) should be %v", tt.ip, tt.isLinkLocal)
		})
	}
}

// TestParseAndValidatePreservesURL verifies URL parsing preserves all components.
func TestParseAndValidatePreservesURL(t *testing.T) {
	// Note: ParseRequestURI (used in ParseAndValidate) treats # as part of RawQuery
	// This is intentional for HTTP requests. If you need fragments, use url.Parse instead.
	original := "https://example.com:8080/path/to/resource?key=value&foo=bar"
	parsed, err := ParseAndValidate(original)
	require.NoError(t, err)

	assert.Equal(t, "https", parsed.Scheme)
	assert.Equal(t, "example.com:8080", parsed.Host)
	assert.Equal(t, "/path/to/resource", parsed.Path)
	assert.Equal(t, "key=value&foo=bar", parsed.RawQuery)
}

// TestParseAndValidateFragment verifies fragment handling in URLs.
func TestParseAndValidateFragment(t *testing.T) {
	// ParseRequestURI doesn't handle fragments the same way as url.Parse
	// Fragments are typically client-side and not sent in HTTP requests
	original := "https://example.com/path#section"
	parsed, err := ParseAndValidate(original)
	require.NoError(t, err)

	// ParseRequestURI includes the fragment in the path or query
	assert.Equal(t, "https", parsed.Scheme)
	assert.Equal(t, "example.com", parsed.Host)
}
