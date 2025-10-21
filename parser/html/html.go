package html

import (
	"context"
	"net/url"
	"regexp"
	"strings"
	"unicode"

	"github.com/joeychilson/websurfer/parser"
	"github.com/joeychilson/websurfer/parser/rules"
	"github.com/microcosm-cc/bluemonday"
	"golang.org/x/net/html"
)

var (
	whitespaceRegex    = regexp.MustCompile(`\s+`)
	tagWhitespaceRegex = regexp.MustCompile(`>\s+<`)

	// Void elements that should not be removed even if empty
	voidElements = map[string]bool{
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

// Parse transforms HTML into LLM-friendly minified HTML.
// Removes scripts, styles, empty elements, divs, and compresses whitespace
// while preserving all semantic content and structure.
// If rules are configured and a URL is in the context, applies matching rules.
// If a URL is in the context, converts relative links to absolute URLs.
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

	if urlStr != "" {
		convertLinksToAbsolute(doc, urlStr)
	}

	var buf strings.Builder
	if err := html.Render(&buf, doc); err != nil {
		return nil, err
	}

	compacted := removeWhitespace(buf.String())

	return []byte(compacted), nil
}

// createSanitizationPolicy creates a policy that keeps structural/semantic elements only.
// Strips scripts, styles, classes, ids, and other non-essential attributes.
func createSanitizationPolicy() *bluemonday.Policy {
	policy := bluemonday.NewPolicy()

	policy.AllowElements("div", "p", "h1", "h2", "h3", "h4", "h5", "h6",
		"header", "footer", "nav", "main",
		"ul", "ol", "li",
		"table", "thead", "tbody", "tr", "td", "th",
		"a", "img", "br", "hr", "input")

	policy.AllowAttrs("href").OnElements("a")
	policy.AllowAttrs("src", "alt").OnElements("img")
	policy.AllowAttrs("colspan", "rowspan").OnElements("td", "th")

	return policy
}

// optimizeHTML performs all HTML optimizations in a single tree traversal.
// Operations: normalize whitespace, remove empty attributes, remove empty nodes, unwrap divs.
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
// Void elements (img, br, hr, etc.) are never considered empty.
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

// removeWhitespace compacts HTML by removing whitespace between tags and newlines,
// but preserves semantic line breaks after block-level elements for LLM-friendly navigation.
func removeWhitespace(htmlStr string) string {
	htmlStr = tagWhitespaceRegex.ReplaceAllString(htmlStr, "><")

	var result strings.Builder
	result.Grow(len(htmlStr))

	for _, line := range strings.Split(htmlStr, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			result.WriteString(trimmed)
		}
	}

	minified := result.String()

	blockElements := []string{
		"</p>", "</h1>", "</h2>", "</h3>", "</h4>", "</h5>", "</h6>",
		"</table>", "</thead>", "</tbody>", "</tfoot>", "</tr>",
		"</ul>", "</ol>", "</li>",
		"</header>", "</footer>", "</nav>", "</main>", "</section>", "</article>", "</aside>",
		"</blockquote>", "</pre>", "</hr>",
	}

	for _, tag := range blockElements {
		minified = strings.ReplaceAll(minified, tag, tag+"\n")
	}

	return minified
}

// convertLinksToAbsolute traverses the HTML tree and converts all relative hrefs and image srcs to absolute URLs.
// Skips javascript:, mailto:, tel:, and # (fragment-only) links for <a> tags.
// Converts all relative image src attributes to absolute URLs for <img> tags.
func convertLinksToAbsolute(n *html.Node, baseURL string) {
	base, err := url.Parse(baseURL)
	if err != nil {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			convertLinksToAbsolute(c, baseURL)
		}
		return
	}

	if n.Type == html.ElementNode {
		if n.Data == "a" {
			for i, attr := range n.Attr {
				if attr.Key == "href" && attr.Val != "" {
					href := strings.TrimSpace(attr.Val)

					if strings.HasPrefix(href, "#") ||
						strings.HasPrefix(href, "javascript:") ||
						strings.HasPrefix(href, "mailto:") ||
						strings.HasPrefix(href, "tel:") {
						continue
					}

					parsed, err := url.Parse(href)
					if err != nil {
						continue
					}

					absolute := base.ResolveReference(parsed)
					n.Attr[i].Val = absolute.String()
				}
			}
		}

		if n.Data == "img" {
			for i, attr := range n.Attr {
				if attr.Key == "src" && attr.Val != "" {
					src := strings.TrimSpace(attr.Val)

					parsed, err := url.Parse(src)
					if err != nil {
						continue
					}

					absolute := base.ResolveReference(parsed)
					n.Attr[i].Val = absolute.String()
				}
			}
		}
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		convertLinksToAbsolute(c, baseURL)
	}
}
