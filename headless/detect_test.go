package headless

import (
	"bytes"
	"testing"
)

func TestNeedsRendering(t *testing.T) {
	tests := []struct {
		name   string
		raw    []byte
		parsed []byte
		want   bool
	}{
		{
			name:   "empty HTML",
			raw:    []byte{},
			parsed: []byte{},
			want:   false,
		},
		{
			name:   "no scripts",
			raw:    []byte(`<html><body><p>Hello</p></body></html>`),
			parsed: []byte{},
			want:   false,
		},
		{
			name:   "scripts but sufficient content",
			raw:    []byte(`<html><script>app()</script><body>content</body></html>`),
			parsed: bytes.Repeat([]byte("x"), 200),
			want:   false,
		},
		{
			name:   "scripts with sparse content",
			raw:    []byte(`<html><script src="app.js"></script><body></body></html>`),
			parsed: []byte("Loading"),
			want:   true,
		},
		{
			name:   "scripts with empty content",
			raw:    []byte(`<html><script>render()</script><body><div id="root"></div></body></html>`),
			parsed: []byte{},
			want:   true,
		},
		{
			name:   "scripts with whitespace only",
			raw:    []byte(`<html><script>init()</script><body>   </body></html>`),
			parsed: []byte("   \n\t  "),
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NeedsRendering(tt.raw, tt.parsed)
			if got != tt.want {
				t.Errorf("NeedsRendering() = %v, want %v", got, tt.want)
			}
		})
	}
}
