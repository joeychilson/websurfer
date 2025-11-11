package rules

// Rule defines a transformation that can be applied to parsed content.
type Rule interface {
	Match(urlStr, contentType string) bool
	Apply(content []byte) []byte
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
