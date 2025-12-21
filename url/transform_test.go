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
			name:  "www github blob to raw",
			input: "https://www.github.com/golang/go/blob/master/src/fmt/print.go",
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
		{
			name:  "arxiv abs to ar5iv html",
			input: "https://arxiv.org/abs/2301.00001",
			want:  "https://ar5iv.labs.arxiv.org/html/2301.00001",
		},
		{
			name:  "arxiv www abs to ar5iv html",
			input: "https://www.arxiv.org/abs/2301.00001",
			want:  "https://ar5iv.labs.arxiv.org/html/2301.00001",
		},
		{
			name:  "arxiv non-abs unchanged",
			input: "https://arxiv.org/list/cs.AI/recent",
			want:  "https://arxiv.org/list/cs.AI/recent",
		},
		{
			name:  "arxiv pdf unchanged",
			input: "https://arxiv.org/pdf/2301.00001.pdf",
			want:  "https://arxiv.org/pdf/2301.00001.pdf",
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
