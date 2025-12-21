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
	case "github.com", "www.github.com":
		return transformGitHub(u)
	case "arxiv.org", "www.arxiv.org":
		return transformArXiv(u)
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

// transformArXiv converts arXiv abstract URLs to ar5iv HTML URLs for easier parsing.
// arxiv.org/abs/2301.00001 → ar5iv.labs.arxiv.org/html/2301.00001
func transformArXiv(u *url.URL) string {
	if !strings.HasPrefix(u.Path, "/abs/") {
		return u.String()
	}

	u.Host = "ar5iv.labs.arxiv.org"
	u.Path = strings.Replace(u.Path, "/abs/", "/html/", 1)
	return u.String()
}
