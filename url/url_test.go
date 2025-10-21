package url

import (
	"strings"
	"testing"
)

func TestNormalize(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		wantErr  bool
	}{
		{
			name:     "removes www prefix",
			input:    "https://www.example.com",
			expected: "https://example.com/",
			wantErr:  false,
		},
		{
			name:     "normalizes http to https",
			input:    "http://example.com",
			expected: "https://example.com/",
			wantErr:  false,
		},
		{
			name:     "removes trailing slash",
			input:    "https://example.com/path/",
			expected: "https://example.com/path",
			wantErr:  false,
		},
		{
			name:     "keeps root trailing slash",
			input:    "https://example.com/",
			expected: "https://example.com/",
			wantErr:  false,
		},
		{
			name:     "removes index.html",
			input:    "https://example.com/index.html",
			expected: "https://example.com/",
			wantErr:  false,
		},
		{
			name:     "removes index.php",
			input:    "https://example.com/path/index.php",
			expected: "https://example.com/path",
			wantErr:  false,
		},
		{
			name:     "removes fragment",
			input:    "https://example.com/page#section",
			expected: "https://example.com/page",
			wantErr:  false,
		},
		{
			name:     "removes default https port",
			input:    "https://example.com:443/path",
			expected: "https://example.com/path",
			wantErr:  false,
		},
		{
			name:     "removes default http port",
			input:    "http://example.com:80/path",
			expected: "https://example.com/path",
			wantErr:  false,
		},
		{
			name:     "keeps non-default port",
			input:    "https://example.com:8080/path",
			expected: "https://example.com:8080/path",
			wantErr:  false,
		},
		{
			name:     "complex normalization",
			input:    "http://www.example.com:80/path/index.html#section",
			expected: "https://example.com/path",
			wantErr:  false,
		},
		{
			name:     "keeps query parameters",
			input:    "https://example.com/page?foo=bar",
			expected: "https://example.com/page?foo=bar",
			wantErr:  false,
		},
		{
			name:     "adds slash to bare domain",
			input:    "https://example.com",
			expected: "https://example.com/",
			wantErr:  false,
		},
		{
			name:    "invalid URL",
			input:   "not a url",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Normalize(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("Normalize(%q) expected error, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Errorf("Normalize(%q) unexpected error: %v", tt.input, err)
				return
			}
			if result != tt.expected {
				t.Errorf("Normalize(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestDeduplicate(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name: "removes http/https duplicates",
			input: []string{
				"http://example.com",
				"https://example.com",
			},
			expected: []string{
				"http://example.com",
			},
		},
		{
			name: "removes www duplicates",
			input: []string{
				"https://example.com",
				"https://www.example.com",
			},
			expected: []string{
				"https://example.com",
			},
		},
		{
			name: "removes trailing slash duplicates",
			input: []string{
				"https://example.com/path",
				"https://example.com/path/",
			},
			expected: []string{
				"https://example.com/path",
			},
		},
		{
			name: "removes index.html duplicates",
			input: []string{
				"https://example.com/",
				"https://example.com/index.html",
			},
			expected: []string{
				"https://example.com/",
			},
		},
		{
			name: "complex deduplication",
			input: []string{
				"http://www.example.com/path/",
				"https://example.com/path",
				"https://www.example.com/path/index.html",
			},
			expected: []string{
				"http://www.example.com/path/",
			},
		},
		{
			name: "preserves different paths",
			input: []string{
				"https://example.com/path1",
				"https://example.com/path2",
			},
			expected: []string{
				"https://example.com/path1",
				"https://example.com/path2",
			},
		},
		{
			name: "preserves order of first occurrence",
			input: []string{
				"https://example.com/page1",
				"https://example.com/page2",
				"http://example.com/page1",
				"https://example.com/page3",
			},
			expected: []string{
				"https://example.com/page1",
				"https://example.com/page2",
				"https://example.com/page3",
			},
		},
		{
			name:     "empty input",
			input:    []string{},
			expected: []string{},
		},
		{
			name: "handles invalid URLs gracefully",
			input: []string{
				"https://example.com/valid",
				"not a url",
				"https://example.com/valid",
			},
			expected: []string{
				"https://example.com/valid",
				"not a url",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Deduplicate(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("Deduplicate() returned %d URLs, want %d", len(result), len(tt.expected))
				t.Errorf("Got: %v", result)
				t.Errorf("Want: %v", tt.expected)
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("Deduplicate()[%d] = %q, want %q", i, result[i], tt.expected[i])
				}
			}
		})
	}
}

func TestIsSame(t *testing.T) {
	tests := []struct {
		name     string
		url1     string
		url2     string
		expected bool
	}{
		{
			name:     "same URL",
			url1:     "https://example.com/path",
			url2:     "https://example.com/path",
			expected: true,
		},
		{
			name:     "http vs https",
			url1:     "http://example.com/path",
			url2:     "https://example.com/path",
			expected: true,
		},
		{
			name:     "www vs non-www",
			url1:     "https://www.example.com/path",
			url2:     "https://example.com/path",
			expected: true,
		},
		{
			name:     "trailing slash",
			url1:     "https://example.com/path/",
			url2:     "https://example.com/path",
			expected: true,
		},
		{
			name:     "different paths",
			url1:     "https://example.com/path1",
			url2:     "https://example.com/path2",
			expected: false,
		},
		{
			name:     "fragment ignored",
			url1:     "https://example.com/path#section1",
			url2:     "https://example.com/path#section2",
			expected: true,
		},
		{
			name:     "invalid url1",
			url1:     "not a url",
			url2:     "https://example.com",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsSame(tt.url1, tt.url2)
			if result != tt.expected {
				t.Errorf("IsSame(%q, %q) = %v, want %v", tt.url1, tt.url2, result, tt.expected)
			}
		})
	}
}

func TestParseAndValidate(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid https URL",
			input:   "https://example.com/path",
			wantErr: false,
		},
		{
			name:    "valid http URL",
			input:   "http://example.com",
			wantErr: false,
		},
		{
			name:    "empty URL",
			input:   "",
			wantErr: true,
			errMsg:  "url cannot be empty",
		},
		{
			name:    "whitespace only",
			input:   "   ",
			wantErr: true,
			errMsg:  "url cannot be empty",
		},
		{
			name:    "relative URL",
			input:   "/path/to/page",
			wantErr: true,
			errMsg:  "url must be absolute",
		},
		{
			name:    "no scheme",
			input:   "example.com",
			wantErr: true,
			errMsg:  "invalid url",
		},
		{
			name:    "invalid scheme",
			input:   "ftp://example.com",
			wantErr: true,
			errMsg:  "url scheme must be http or https",
		},
		{
			name:    "file scheme",
			input:   "file:///etc/passwd",
			wantErr: true,
			errMsg:  "url must be absolute",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseAndValidate(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseAndValidate(%q) expected error, got nil", tt.input)
					return
				}
				if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("ParseAndValidate(%q) error = %q, want to contain %q", tt.input, err.Error(), tt.errMsg)
				}
				return
			}
			if err != nil {
				t.Errorf("ParseAndValidate(%q) unexpected error: %v", tt.input, err)
				return
			}
			if result == nil {
				t.Errorf("ParseAndValidate(%q) returned nil URL", tt.input)
			}
		})
	}
}

func TestValidateExternal(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid external URL",
			input:   "https://example.com",
			wantErr: false,
		},
		{
			name:    "localhost",
			input:   "https://localhost",
			wantErr: true,
			errMsg:  "private IP",
		},
		{
			name:    "127.0.0.1",
			input:   "https://127.0.0.1",
			wantErr: true,
			errMsg:  "private IP",
		},
		{
			name:    "private IP 10.x",
			input:   "https://10.0.0.1",
			wantErr: true,
			errMsg:  "private IP",
		},
		{
			name:    "private IP 192.168.x",
			input:   "https://192.168.1.1",
			wantErr: true,
			errMsg:  "private IP",
		},
		{
			name:    "private IP 172.16.x",
			input:   "https://172.16.0.1",
			wantErr: true,
			errMsg:  "private IP",
		},
		{
			name:    "IPv6 loopback",
			input:   "https://[::1]",
			wantErr: true,
			errMsg:  "private IP",
		},
		{
			name:    "invalid URL",
			input:   "not a url",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateExternal(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateExternal(%q) expected error, got nil", tt.input)
					return
				}
				if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("ValidateExternal(%q) error = %q, want to contain %q", tt.input, err.Error(), tt.errMsg)
				}
				return
			}
			if err != nil {
				t.Errorf("ValidateExternal(%q) unexpected error: %v", tt.input, err)
			}
		})
	}
}

func TestIsSameDomain(t *testing.T) {
	tests := []struct {
		name     string
		url1     string
		url2     string
		expected bool
	}{
		{
			name:     "same domain",
			url1:     "https://example.com/path1",
			url2:     "https://example.com/path2",
			expected: true,
		},
		{
			name:     "www ignored",
			url1:     "https://www.example.com",
			url2:     "https://example.com",
			expected: true,
		},
		{
			name:     "different domains",
			url1:     "https://example.com",
			url2:     "https://other.com",
			expected: false,
		},
		{
			name:     "subdomain is different",
			url1:     "https://sub.example.com",
			url2:     "https://example.com",
			expected: false,
		},
		{
			name:     "different subdomains",
			url1:     "https://sub1.example.com",
			url2:     "https://sub2.example.com",
			expected: false,
		},
		{
			name:     "invalid url1",
			url1:     "not a url",
			url2:     "https://example.com",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsSameDomain(tt.url1, tt.url2)
			if result != tt.expected {
				t.Errorf("IsSameDomain(%q, %q) = %v, want %v", tt.url1, tt.url2, result, tt.expected)
			}
		})
	}
}

func TestIsSameSubdomain(t *testing.T) {
	tests := []struct {
		name     string
		url1     string
		url2     string
		expected bool
	}{
		{
			name:     "exact match",
			url1:     "https://example.com/path1",
			url2:     "https://example.com/path2",
			expected: true,
		},
		{
			name:     "www treated as different subdomain",
			url1:     "https://www.example.com",
			url2:     "https://example.com",
			expected: false,
		},
		{
			name:     "same subdomain",
			url1:     "https://sub.example.com/path1",
			url2:     "https://sub.example.com/path2",
			expected: true,
		},
		{
			name:     "different subdomains",
			url1:     "https://sub1.example.com",
			url2:     "https://sub2.example.com",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsSameSubdomain(tt.url1, tt.url2)
			if result != tt.expected {
				t.Errorf("IsSameSubdomain(%q, %q) = %v, want %v", tt.url1, tt.url2, result, tt.expected)
			}
		})
	}
}

func TestIsSameBaseDomain(t *testing.T) {
	tests := []struct {
		name     string
		url1     string
		url2     string
		expected bool
	}{
		{
			name:     "different subdomains same base",
			url1:     "https://blog.example.com/page",
			url2:     "https://docs.example.com/doc",
			expected: true,
		},
		{
			name:     "subdomain and root",
			url1:     "https://api.github.com/users",
			url2:     "https://github.com",
			expected: true,
		},
		{
			name:     "www and subdomain",
			url1:     "https://www.example.com",
			url2:     "https://blog.example.com",
			expected: true,
		},
		{
			name:     "multiple levels same base",
			url1:     "https://api.v2.example.com",
			url2:     "https://web.app.example.com",
			expected: true,
		},
		{
			name:     "completely different domains",
			url1:     "https://example.com",
			url2:     "https://other.com",
			expected: false,
		},
		{
			name:     "similar but different",
			url1:     "https://example.com",
			url2:     "https://example.org",
			expected: false,
		},
		{
			name:     "co.uk same base",
			url1:     "https://blog.example.co.uk",
			url2:     "https://www.example.co.uk",
			expected: true,
		},
		{
			name:     "com.au same base",
			url1:     "https://shop.example.com.au",
			url2:     "https://example.com.au",
			expected: true,
		},
		{
			name:     "gov.uk different",
			url1:     "https://example.gov.uk",
			url2:     "https://other.gov.uk",
			expected: false,
		},
		{
			name:     "same exact URL",
			url1:     "https://example.com/page",
			url2:     "https://example.com/other",
			expected: true,
		},
		{
			name:     "localhost",
			url1:     "http://localhost:8080",
			url2:     "http://localhost:3000",
			expected: true,
		},
		{
			name:     "IP addresses same",
			url1:     "http://192.168.1.1",
			url2:     "http://192.168.1.1:8080",
			expected: true,
		},
		{
			name:     "IP addresses different",
			url1:     "http://192.168.1.1",
			url2:     "http://192.168.1.2",
			expected: false,
		},
		{
			name:     "invalid URL 1",
			url1:     "not-a-url",
			url2:     "https://example.com",
			expected: false,
		},
		{
			name:     "invalid URL 2",
			url1:     "https://example.com",
			url2:     "invalid",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsSameBaseDomain(tt.url1, tt.url2)
			if result != tt.expected {
				t.Errorf("IsSameBaseDomain(%q, %q) = %v, want %v", tt.url1, tt.url2, result, tt.expected)
			}
		})
	}
}

// Helper function to check if a string contains a substring (case-insensitive)
func contains(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
