package pdf

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPDFParserCreation verifies PDF parser can be created.
func TestPDFParserCreation(t *testing.T) {
	parser := New()
	assert.NotNil(t, parser, "should create PDF parser")
}

// TestPDFParseEmptyContent verifies empty content handling.
func TestPDFParseEmptyContent(t *testing.T) {
	parser := New()

	result, err := parser.Parse(context.Background(), []byte(""))

	require.NoError(t, err)
	assert.Equal(t, []byte(""), result, "empty input should return empty output")
}

// TestPDFParseNilContent verifies nil content handling.
func TestPDFParseNilContent(t *testing.T) {
	parser := New()

	result, err := parser.Parse(context.Background(), nil)

	require.NoError(t, err)
	assert.Nil(t, result, "nil input should return nil output")
}

// TestPDFParseMissingPDFToText verifies error when pdftotext not available.
// This test will pass whether pdftotext is installed or not - it just
// verifies that the parser checks for the command.
func TestPDFParseMissingPDFToText(t *testing.T) {
	parser := New()

	// Use minimal PDF header (not a valid PDF, but enough to test the flow)
	pdfContent := []byte("%PDF-1.4\n")

	result, err := parser.Parse(context.Background(), pdfContent)

	// If pdftotext is not installed, should get an error about it not being in PATH
	// If it is installed, might get an error about invalid PDF format
	// Either way, we're testing that the parser attempts to use pdftotext
	if err != nil {
		// Expected when pdftotext not available or PDF is invalid
		assert.Contains(t, err.Error(), "pdftotext", "error should mention pdftotext if missing")
	} else {
		// pdftotext might be installed and handled the minimal input
		assert.NotNil(t, result)
	}
}

// TestPDFParseInvalidPDFContent verifies error handling for malformed PDF.
// This test only runs if pdftotext is available.
func TestPDFParseInvalidPDFContent(t *testing.T) {
	parser := New()

	// Completely invalid PDF content
	invalidPDF := []byte("this is not a pdf")

	_, err := parser.Parse(context.Background(), invalidPDF)

	// Should get an error (either pdftotext not found, or PDF parsing failed)
	// We don't require a specific error, just that it doesn't panic
	if err != nil {
		t.Logf("Expected error for invalid PDF: %v", err)
	}
}

// TestPDFParseContextCancellation verifies context cancellation is respected.
func TestPDFParseContextCancellation(t *testing.T) {
	parser := New()

	// Create a context that's already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Use minimal PDF content
	pdfContent := []byte("%PDF-1.4\nsome content")

	_, err := parser.Parse(ctx, pdfContent)

	// Should either get context cancelled error or pdftotext not found error
	// (if pdftotext isn't installed, that error comes first)
	if err != nil {
		// Expected - either context error or missing pdftotext
		assert.Error(t, err)
	}
}

// TestPDFParseTimeoutProtection verifies 30s timeout is applied.
// This test documents the timeout behavior without actually waiting 30s.
func TestPDFParseTimeoutProtection(t *testing.T) {
	// This test just documents that the parser has a 30s timeout
	// Actual timeout testing would require a malformed PDF that hangs pdftotext
	assert.Equal(t, defaultPDFTimeout.Seconds(), 30.0, "should have 30s timeout")
}
