package parser_test

import (
	"context"
	"testing"

	"github.com/joeychilson/websurfer/parser"
	"github.com/joeychilson/websurfer/parser/html"
)

func TestRegistry_Register(t *testing.T) {
	registry := parser.New()
	htmlParser := html.New()

	registry.Register([]string{"text/html", "application/xhtml+xml"}, htmlParser)

	if !registry.HasParser("text/html") {
		t.Error("HasParser() should return true for text/html")
	}
	if !registry.HasParser("application/xhtml+xml") {
		t.Error("HasParser() should return true for application/xhtml+xml")
	}
	if registry.HasParser("text/plain") {
		t.Error("HasParser() should return false for text/plain")
	}
}

func TestRegistry_Parse(t *testing.T) {
	registry := parser.New()
	htmlParser := html.New()
	registry.Register([]string{"text/html"}, htmlParser)

	ctx := context.Background()

	t.Run("parses registered content type", func(t *testing.T) {
		input := []byte("<html><body><p>Test</p></body></html>")
		output, err := registry.Parse(ctx, "text/html", input)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		if len(output) == 0 {
			t.Error("Parse() should return non-empty output")
		}
	})

	t.Run("normalizes content type with charset", func(t *testing.T) {
		input := []byte("<html><body><p>Test</p></body></html>")
		output, err := registry.Parse(ctx, "text/html; charset=utf-8", input)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		if len(output) == 0 {
			t.Error("Parse() should return non-empty output")
		}
	})

	t.Run("normalizes content type case", func(t *testing.T) {
		input := []byte("<html><body><p>Test</p></body></html>")
		output, err := registry.Parse(ctx, "TEXT/HTML", input)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		if len(output) == 0 {
			t.Error("Parse() should return non-empty output")
		}
	})

	t.Run("returns unchanged content for unregistered type", func(t *testing.T) {
		input := []byte(`{"key": "value"}`)
		output, err := registry.Parse(ctx, "application/json", input)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		if string(output) != string(input) {
			t.Errorf("Parse() should return unchanged content for unregistered type, got %s", output)
		}
	})

	t.Run("returns unchanged content for empty content type", func(t *testing.T) {
		input := []byte("test")
		output, err := registry.Parse(ctx, "", input)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		if string(output) != string(input) {
			t.Errorf("Parse() should return unchanged content for empty content type")
		}
	})

	t.Run("returns unchanged content for empty input", func(t *testing.T) {
		input := []byte("")
		output, err := registry.Parse(ctx, "text/html", input)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		if len(output) != 0 {
			t.Errorf("Parse() should return empty output for empty input")
		}
	})
}

func TestRegistry_HasParser(t *testing.T) {
	registry := parser.New()
	htmlParser := html.New()

	registry.Register([]string{"text/html"}, htmlParser)

	tests := []struct {
		contentType string
		want        bool
	}{
		{"text/html", true},
		{"text/html; charset=utf-8", true},
		{"TEXT/HTML", true},
		{"application/json", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.contentType, func(t *testing.T) {
			got := registry.HasParser(tt.contentType)
			if got != tt.want {
				t.Errorf("HasParser(%q) = %v, want %v", tt.contentType, got, tt.want)
			}
		})
	}
}

// TestNormalizeContentType is tested via the public API (HasParser, Parse)
// since normalizeContentType is now package-private
func TestNew(t *testing.T) {
	registry := parser.New()
	if registry == nil {
		t.Fatal("New() should return non-nil registry")
	}
}
