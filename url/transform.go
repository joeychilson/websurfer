package url

import (
	"net/url"
	"strings"
)

// Transform converts URLs to their optimal fetch format.
// For example, GitHub blob URLs are converted to raw URLs for direct content access.
func Transform(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	switch u.Host {
	case "github.com":
		return transformGitHub(u)
	}

	return rawURL
}

// transformGitHub converts GitHub blob URLs to raw.githubusercontent.com URLs.
// github.com/owner/repo/blob/branch/path → raw.githubusercontent.com/owner/repo/branch/path
func transformGitHub(u *url.URL) string {
	if !strings.Contains(u.Path, "/blob/") {
		return u.String()
	}

	u.Host = "raw.githubusercontent.com"
	u.Path = strings.Replace(u.Path, "/blob/", "/", 1)
	return u.String()
}
