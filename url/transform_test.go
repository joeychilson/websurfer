package url

import "testing"

func TestTransform(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "github blob to raw",
			input: "https://github.com/golang/go/blob/master/src/fmt/print.go",
			want:  "https://raw.githubusercontent.com/golang/go/master/src/fmt/print.go",
		},
		{
			name:  "github non-blob unchanged",
			input: "https://github.com/golang/go",
			want:  "https://github.com/golang/go",
		},
		{
			name:  "non-github unchanged",
			input: "https://example.com/path/to/file",
			want:  "https://example.com/path/to/file",
		},
		{
			name:  "raw github unchanged",
			input: "https://raw.githubusercontent.com/golang/go/master/src/fmt/print.go",
			want:  "https://raw.githubusercontent.com/golang/go/master/src/fmt/print.go",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Transform(tt.input)
			if got != tt.want {
				t.Errorf("Transform(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
