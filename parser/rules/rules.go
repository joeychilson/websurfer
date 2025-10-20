package rules

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/joeychilson/websurfer/parser"
)

// Rule defines a transformation that can be applied to parsed content.
// Rules are URL and content-type aware, applying only when conditions match.
type Rule interface {
	// Match returns true if this rule should be applied
	Match(urlStr, contentType string) bool
	// Apply applies the rule's transformations to the content
	Apply(content []byte) []byte
	// Name returns the rule's name for logging/debugging
	Name() string
}

// RuleChain manages a collection of rules and applies them in order.
type RuleChain struct {
	rules []Rule
}

// NewRuleChain creates a new rule chain with the given rules.
func NewRuleChain(rules ...Rule) *RuleChain {
	return &RuleChain{
		rules: rules,
	}
}

// Add adds a rule to the chain.
func (rc *RuleChain) Add(rule Rule) {
	rc.rules = append(rc.rules, rule)
}

// Apply applies all matching rules to the content for the given URL and content type.
// Rules are applied in the order they were added.
func (rc *RuleChain) Apply(urlStr, contentType string, content []byte) []byte {
	if len(rc.rules) == 0 {
		return content
	}

	result := content
	for _, rule := range rc.rules {
		if rule.Match(urlStr, contentType) {
			result = rule.Apply(result)
		}
	}

	return result
}

// DomainRule is a rule that matches URLs by domain and optionally by content-type.
type DomainRule struct {
	domain      string
	contentType string // empty means match all content types
	transform   func([]byte) []byte
	name        string
}

// NewDomainRule creates a rule that matches a specific domain.
// contentType can be empty to match all content types, or specify like "text/html".
func NewDomainRule(domain, contentType, name string, transform func([]byte) []byte) *DomainRule {
	return &DomainRule{
		domain:      strings.ToLower(domain),
		contentType: parser.NormalizeContentType(contentType),
		transform:   transform,
		name:        name,
	}
}

// Match checks if the URL's domain and content-type match this rule.
func (r *DomainRule) Match(urlStr, contentType string) bool {
	if r.contentType != "" && parser.NormalizeContentType(contentType) != r.contentType {
		return false
	}

	u, err := url.Parse(urlStr)
	if err != nil {
		return false
	}

	host := strings.ToLower(u.Host)
	return host == r.domain || strings.HasSuffix(host, "."+r.domain)
}

// Apply applies the transformation function.
func (r *DomainRule) Apply(content []byte) []byte {
	return r.transform(content)
}

// Name returns the rule's name.
func (r *DomainRule) Name() string {
	return r.name
}

// PathRule is a rule that matches URLs by path pattern and optionally by content-type.
type PathRule struct {
	pathRegex   *regexp.Regexp
	contentType string
	transform   func([]byte) []byte
	name        string
}

// NewPathRule creates a rule that matches URL paths using a regex pattern.
// contentType can be empty to match all content types.
func NewPathRule(pathPattern, contentType, name string, transform func([]byte) []byte) (*PathRule, error) {
	re, err := regexp.Compile(pathPattern)
	if err != nil {
		return nil, err
	}

	return &PathRule{
		pathRegex:   re,
		contentType: parser.NormalizeContentType(contentType),
		transform:   transform,
		name:        name,
	}, nil
}

// Match checks if the URL's path and content-type match this rule.
func (r *PathRule) Match(urlStr, contentType string) bool {
	if r.contentType != "" && parser.NormalizeContentType(contentType) != r.contentType {
		return false
	}

	u, err := url.Parse(urlStr)
	if err != nil {
		return false
	}

	return r.pathRegex.MatchString(u.Path)
}

// Apply applies the transformation function.
func (r *PathRule) Apply(content []byte) []byte {
	return r.transform(content)
}

// Name returns the rule's name.
func (r *PathRule) Name() string {
	return r.name
}

// CompositeRule combines domain, path, and content-type matching.
type CompositeRule struct {
	domain      string
	pathRegex   *regexp.Regexp
	contentType string
	transform   func([]byte) []byte
	name        string
}

// NewCompositeRule creates a rule that matches domain, path pattern, and content-type.
// pathPattern can be empty to match all paths.
// contentType can be empty to match all content types.
func NewCompositeRule(domain, pathPattern, contentType, name string, transform func([]byte) []byte) (*CompositeRule, error) {
	var re *regexp.Regexp
	var err error

	if pathPattern != "" {
		re, err = regexp.Compile(pathPattern)
		if err != nil {
			return nil, err
		}
	}

	return &CompositeRule{
		domain:      strings.ToLower(domain),
		pathRegex:   re,
		contentType: parser.NormalizeContentType(contentType),
		transform:   transform,
		name:        name,
	}, nil
}

// Match checks if domain, path, and content-type all match.
func (r *CompositeRule) Match(urlStr, contentType string) bool {
	if r.contentType != "" && parser.NormalizeContentType(contentType) != r.contentType {
		return false
	}

	u, err := url.Parse(urlStr)
	if err != nil {
		return false
	}

	host := strings.ToLower(u.Host)
	domainMatch := host == r.domain || strings.HasSuffix(host, "."+r.domain)

	if !domainMatch {
		return false
	}

	if r.pathRegex != nil {
		return r.pathRegex.MatchString(u.Path)
	}

	return true
}

// Apply applies the transformation function.
func (r *CompositeRule) Apply(content []byte) []byte {
	return r.transform(content)
}

// Name returns the rule's name.
func (r *CompositeRule) Name() string {
	return r.name
}
