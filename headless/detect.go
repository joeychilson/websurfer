package headless

import "bytes"

// NeedsRendering returns true if the page likely needs JavaScript rendering.
func NeedsRendering(rawHTML []byte, parsedContent []byte) bool {
	// No HTML, nothing to do
	if len(rawHTML) == 0 {
		return false
	}

	// No scripts means no JS rendering possible
	if !bytes.Contains(rawHTML, []byte("<script")) {
		return false
	}

	// Got enough content, no need for headless
	contentLen := len(bytes.TrimSpace(parsedContent))
	if contentLen >= 200 {
		return false
	}

	return true
}
