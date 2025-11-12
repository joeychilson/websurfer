package rules

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// mockRule is a simple rule for testing
type mockRule struct {
	name      string
	shouldMatch bool
	transform   string
}

func (m *mockRule) Match(urlStr, contentType string) bool {
	return m.shouldMatch
}

func (m *mockRule) Apply(content []byte) []byte {
	return []byte(m.transform)
}

func (m *mockRule) Name() string {
	return m.name
}

// TestRuleChainNew verifies rule chain can be created.
func TestRuleChainNew(t *testing.T) {
	rule1 := &mockRule{name: "rule1", shouldMatch: true, transform: "transformed"}
	rule2 := &mockRule{name: "rule2", shouldMatch: true, transform: "transformed"}

	chain := NewRuleChain(rule1, rule2)

	assert.NotNil(t, chain)
	assert.Len(t, chain.rules, 2)
}

// TestRuleChainNewEmpty verifies empty rule chain can be created.
func TestRuleChainNewEmpty(t *testing.T) {
	chain := NewRuleChain()

	assert.NotNil(t, chain)
	assert.Len(t, chain.rules, 0)
}

// TestRuleChainAdd verifies rules can be added to chain.
func TestRuleChainAdd(t *testing.T) {
	chain := NewRuleChain()
	rule := &mockRule{name: "rule1", shouldMatch: true, transform: "transformed"}

	chain.Add(rule)

	assert.Len(t, chain.rules, 1)
}

// TestRuleChainApplyNoRules verifies empty chain returns original content.
func TestRuleChainApplyNoRules(t *testing.T) {
	chain := NewRuleChain()
	content := []byte("original content")

	result := chain.Apply("https://example.com", "text/html", content)

	assert.Equal(t, content, result)
}

// TestRuleChainApplyMatchingRule verifies matching rules are applied.
func TestRuleChainApplyMatchingRule(t *testing.T) {
	rule := &mockRule{
		name:        "test-rule",
		shouldMatch: true,
		transform:   "transformed",
	}
	chain := NewRuleChain(rule)
	content := []byte("original")

	result := chain.Apply("https://example.com", "text/html", content)

	assert.Equal(t, []byte("transformed"), result)
}

// TestRuleChainApplyNonMatchingRule verifies non-matching rules are skipped.
func TestRuleChainApplyNonMatchingRule(t *testing.T) {
	rule := &mockRule{
		name:        "test-rule",
		shouldMatch: false, // Won't match
		transform:   "should not see this",
	}
	chain := NewRuleChain(rule)
	content := []byte("original")

	result := chain.Apply("https://example.com", "text/html", content)

	assert.Equal(t, []byte("original"), result, "non-matching rule should not transform")
}

// TestRuleChainApplyMultipleRules verifies rules are applied in order.
func TestRuleChainApplyMultipleRules(t *testing.T) {
	rule1 := &mockRule{
		name:        "rule1",
		shouldMatch: true,
		transform:   "step1",
	}
	rule2 := &mockRule{
		name:        "rule2",
		shouldMatch: true,
		transform:   "step2",
	}
	chain := NewRuleChain(rule1, rule2)
	content := []byte("original")

	result := chain.Apply("https://example.com", "text/html", content)

	// Each rule transforms the output of the previous
	assert.Equal(t, []byte("step2"), result)
}

// TestRuleChainApplyMixedMatching verifies only matching rules transform.
func TestRuleChainApplyMixedMatching(t *testing.T) {
	rule1 := &mockRule{
		name:        "rule1",
		shouldMatch: true,
		transform:   "first",
	}
	rule2 := &mockRule{
		name:        "rule2",
		shouldMatch: false, // Won't match
		transform:   "second",
	}
	rule3 := &mockRule{
		name:        "rule3",
		shouldMatch: true,
		transform:   "third",
	}
	chain := NewRuleChain(rule1, rule2, rule3)
	content := []byte("original")

	result := chain.Apply("https://example.com", "text/html", content)

	// rule1 transforms to "first", rule2 skipped, rule3 transforms to "third"
	assert.Equal(t, []byte("third"), result)
}
