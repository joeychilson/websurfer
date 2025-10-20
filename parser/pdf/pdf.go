package pdf

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
)

// Parser converts PDF content to plain text using pdftotext.
type Parser struct{}

// New creates a new PDF parser.
func New() *Parser {
	return &Parser{}
}

// Parse converts PDF bytes to plain text using pdftotext with -layout -nopgbrk flags.
// The -layout flag maintains the original physical layout of the text.
// The -nopgbrk flag disables page break characters in the output.
func (p *Parser) Parse(ctx context.Context, content []byte) ([]byte, error) {
	if len(content) == 0 {
		return content, nil
	}

	if _, err := exec.LookPath("pdftotext"); err != nil {
		return nil, fmt.Errorf("pdftotext not found in PATH: %w", err)
	}

	tmpFile, err := os.CreateTemp("", "websurfer-*.pdf")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	if _, err := tmpFile.Write(content); err != nil {
		return nil, fmt.Errorf("failed to write PDF to temp file: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return nil, fmt.Errorf("failed to close temp file: %w", err)
	}

	cmd := exec.CommandContext(ctx, "pdftotext", "-layout", "-nopgbrk", tmpFile.Name(), "-")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("pdftotext failed: %w (stderr: %s)", err, stderr.String())
	}

	return stdout.Bytes(), nil
}
