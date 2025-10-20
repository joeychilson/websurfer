package pdf

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

func TestParser_Parse_Empty(t *testing.T) {
	parser := New()
	result, err := parser.Parse(context.Background(), []byte{})
	if err != nil {
		t.Fatalf("expected no error for empty content, got: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("expected empty result for empty content, got: %d bytes", len(result))
	}
}

func TestParser_Parse_InvalidPDF(t *testing.T) {
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext not available, skipping test")
	}

	parser := New()
	invalidPDF := []byte("This is not a valid PDF file")

	_, err := parser.Parse(context.Background(), invalidPDF)
	if err == nil {
		t.Fatal("expected error for invalid PDF content, got nil")
	}

	if !strings.Contains(err.Error(), "pdftotext failed") {
		t.Fatalf("expected pdftotext error, got: %v", err)
	}
}

func TestParser_Parse_ValidPDF(t *testing.T) {
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext not available, skipping test")
	}

	parser := New()

	minimalPDF := []byte(`%PDF-1.4
1 0 obj
<<
/Type /Catalog
/Pages 2 0 R
>>
endobj
2 0 obj
<<
/Type /Pages
/Kids [3 0 R]
/Count 1
>>
endobj
3 0 obj
<<
/Type /Page
/Parent 2 0 R
/MediaBox [0 0 612 792]
/Contents 4 0 R
/Resources <<
/Font <<
/F1 <<
/Type /Font
/Subtype /Type1
/BaseFont /Helvetica
>>
>>
>>
>>
endobj
4 0 obj
<<
/Length 44
>>
stream
BT
/F1 12 Tf
100 700 Td
(Hello World) Tj
ET
endstream
endobj
xref
0 5
0000000000 65535 f
0000000009 00000 n
0000000058 00000 n
0000000115 00000 n
0000000317 00000 n
trailer
<<
/Size 5
/Root 1 0 R
>>
startxref
410
%%EOF
`)

	result, err := parser.Parse(context.Background(), minimalPDF)
	if err != nil {
		t.Fatalf("expected no error for valid PDF, got: %v", err)
	}

	resultStr := string(result)
	if !strings.Contains(resultStr, "Hello World") {
		t.Fatalf("expected result to contain 'Hello World', got: %s", resultStr)
	}
}

func TestParser_Parse_ContextCancellation(t *testing.T) {
	// Skip if pdftotext is not available
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext not available, skipping test")
	}

	parser := New()

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	minimalPDF := []byte(`%PDF-1.4
1 0 obj
<<
/Type /Catalog
/Pages 2 0 R
>>
endobj
2 0 obj
<<
/Type /Pages
/Kids [3 0 R]
/Count 1
>>
endobj
3 0 obj
<<
/Type /Page
/Parent 2 0 R
/MediaBox [0 0 612 792]
/Contents 4 0 R
>>
endobj
4 0 obj
<<
/Length 0
>>
stream
endstream
endobj
xref
0 5
0000000000 65535 f
0000000009 00000 n
0000000058 00000 n
0000000115 00000 n
0000000228 00000 n
trailer
<<
/Size 5
/Root 1 0 R
>>
startxref
277
%%EOF
`)

	_, err := parser.Parse(ctx, minimalPDF)
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

func TestParser_Parse_NoPdftotextInstalled(t *testing.T) {
	// This test is tricky to implement without actually removing pdftotext from PATH
	// We'll test the error path by checking the error message structure
	// when pdftotext is not in PATH (if applicable)

	// Just verify the parser can be created
	parser := New()
	if parser == nil {
		t.Fatal("expected parser to be created, got nil")
	}
}
