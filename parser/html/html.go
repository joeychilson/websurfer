package html

import (
	"context"
	"regexp"
	"strings"
	"unicode"

	"github.com/JohannesKaufmann/html-to-markdown/v2/converter"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/base"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/commonmark"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/table"
	"github.com/joeychilson/websurfer/parser"
	"github.com/joeychilson/websurfer/parser/rules"
	"github.com/microcosm-cc/bluemonday"
	"golang.org/x/net/html"
)

var (
	whitespaceRegex = regexp.MustCompile(`\s+`)
	voidElements    = map[string]bool{
		"img": true, "br": true, "hr": true, "input": true,
		"meta": true, "link": true, "area": true, "base": true,
		"col": true, "embed": true, "param": true, "source": true,
		"track": true, "wbr": true,
	}
)

// Parser cleans HTML content into a minified format optimized for LLM consumption.
type Parser struct {
	policy *bluemonday.Policy
	rules  *rules.RuleChain
}

// Option is a functional option for configuring the Parser.
type Option func(*Parser)

// WithRules adds custom rules to the parser.
func WithRules(ruleList ...rules.Rule) Option {
	return func(p *Parser) {
		if p.rules == nil {
			p.rules = rules.NewRuleChain()
		}
		for _, rule := range ruleList {
			p.rules.Add(rule)
		}
	}
}

// New creates a new HTML parser with default sanitization settings.
func New(opts ...Option) *Parser {
	p := &Parser{
		policy: createSanitizationPolicy(),
	}

	for _, opt := range opts {
		opt(p)
	}

	return p
}

// Parse transforms HTML into LLM-friendly Markdown.
func (p *Parser) Parse(ctx context.Context, content []byte) ([]byte, error) {
	if len(content) == 0 {
		return content, nil
	}

	result := content
	urlStr := parser.GetURL(ctx)
	if p.rules != nil {
		if urlStr != "" {
			result = p.rules.Apply(urlStr, "text/html", result)
		}
	}

	sanitized := p.policy.Sanitize(string(result))

	doc, err := html.Parse(strings.NewReader(sanitized))
	if err != nil {
		return nil, err
	}

	optimizeHTML(doc)

	opts := []converter.ConvertOptionFunc{}
	if urlStr != "" {
		opts = append(opts, converter.WithDomain(urlStr))
	}

	conv := converter.NewConverter(
		converter.WithPlugins(
			base.NewBasePlugin(),
			commonmark.NewCommonmarkPlugin(),
			table.NewTablePlugin(),
		),
	)

	markdownBytes, err := conv.ConvertNode(doc, opts...)
	if err != nil {
		return nil, err
	}

	return markdownBytes, nil
}

// createSanitizationPolicy creates a policy that keeps structural/semantic elements only.
func createSanitizationPolicy() *bluemonday.Policy {
	policy := bluemonday.NewPolicy()

	policy.AllowElements("div", "p", "h1", "h2", "h3", "h4", "h5", "h6",
		"main",
		"ul", "ol", "li",
		"table", "thead", "tbody", "tr", "td", "th",
		"a", "br", "hr")

	policy.AllowAttrs("href").OnElements("a")
	policy.AllowAttrs("colspan", "rowspan").OnElements("td", "th")

	return policy
}

// optimizeHTML performs all HTML optimizations in a single tree traversal.
func optimizeHTML(n *html.Node) {
	for c := n.FirstChild; c != nil; {
		next := c.NextSibling
		optimizeHTML(c)
		c = next
	}

	if n.Type == html.TextNode {
		data := n.Data
		normalized := whitespaceRegex.ReplaceAllString(data, " ")

		if normalized != " " {
			trimmed := strings.TrimSpace(normalized)
			if trimmed != "" {
				if data != "" && unicode.IsSpace(rune(data[0])) {
					trimmed = " " + trimmed
				}
				if data != "" && unicode.IsSpace(rune(data[len(data)-1])) {
					trimmed = trimmed + " "
				}
				normalized = trimmed
			}
		}
		n.Data = normalized
	}

	if n.Type == html.ElementNode && len(n.Attr) > 0 {
		filtered := n.Attr[:0]
		for _, attr := range n.Attr {
			if attr.Val != "" {
				filtered = append(filtered, attr)
			}
		}
		n.Attr = filtered
	}

	if n.Type == html.ElementNode && isEmptyNode(n) && n.Parent != nil {
		n.Parent.RemoveChild(n)
		return
	}

	if n.Type == html.ElementNode && n.Data == "div" && n.Parent != nil {
		for c := n.FirstChild; c != nil; {
			next := c.NextSibling
			n.RemoveChild(c)
			n.Parent.InsertBefore(c, n)
			c = next
		}
		n.Parent.RemoveChild(n)
	}
}

// isEmptyNode checks if a node is empty (no text content, only whitespace and empty children).
func isEmptyNode(n *html.Node) bool {
	if voidElements[n.Data] {
		return false
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		switch c.Type {
		case html.TextNode:
			if strings.TrimSpace(c.Data) != "" {
				return false
			}
		case html.ElementNode:
			if !isEmptyNode(c) {
				return false
			}
		}
	}

	return true
}
