package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/joeychilson/websurfer/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidateRequestValid verifies valid requests pass validation.
func TestValidateRequestValid(t *testing.T) {
	c, _ := client.New(nil)
	defer c.Close()
	s, _ := New(c, nil, nil)

	req := &FetchRequest{
		URL:       "https://example.com",
		MaxTokens: 1000,
		Offset:    0,
	}

	err := s.validateRequest(req)
	assert.NoError(t, err)
}

// TestValidateRequestNil verifies nil request is rejected.
func TestValidateRequestNil(t *testing.T) {
	c, _ := client.New(nil)
	defer c.Close()
	s, _ := New(c, nil, nil)

	err := s.validateRequest(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be nil")
}

// TestValidateRequestInvalidURL verifies invalid URLs are rejected.
func TestValidateRequestInvalidURL(t *testing.T) {
	c, _ := client.New(nil)
	defer c.Close()
	s, _ := New(c, nil, nil)

	tests := []string{
		"not-a-url",
		"ftp://example.com", // Not http/https
		"",
		"http://127.0.0.1", // Private IP
	}

	for _, url := range tests {
		req := &FetchRequest{URL: url}
		err := s.validateRequest(req)
		assert.Error(t, err, "should reject: %s", url)
	}
}

// TestValidateRequestNegativeMaxTokens verifies negative max_tokens is rejected.
func TestValidateRequestNegativeMaxTokens(t *testing.T) {
	c, _ := client.New(nil)
	defer c.Close()
	s, _ := New(c, nil, nil)

	req := &FetchRequest{
		URL:       "https://example.com",
		MaxTokens: -1,
	}

	err := s.validateRequest(req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "max_tokens")
}

// TestValidateRequestNegativeOffset verifies negative offset is rejected.
func TestValidateRequestNegativeOffset(t *testing.T) {
	c, _ := client.New(nil)
	defer c.Close()
	s, _ := New(c, nil, nil)

	req := &FetchRequest{
		URL:    "https://example.com",
		Offset: -1,
	}

	err := s.validateRequest(req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "offset")
}

// TestHandleHealthEndpoint verifies /health endpoint works.
func TestHandleHealthEndpoint(t *testing.T) {
	c, err := client.New(nil)
	require.NoError(t, err)
	defer c.Close()

	s, err := New(c, nil, nil)
	require.NoError(t, err)

	router := s.Router()

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "application/json")

	var health map[string]string
	err = json.NewDecoder(w.Body).Decode(&health)
	require.NoError(t, err)
	assert.Equal(t, "ok", health["status"])
	assert.NotEmpty(t, health["time"])
}

// TestHandleFetchInvalidJSON verifies invalid JSON is rejected.
func TestHandleFetchInvalidJSON(t *testing.T) {
	c, err := client.New(nil)
	require.NoError(t, err)
	defer c.Close()

	s, err := New(c, nil, nil)
	require.NoError(t, err)

	router := s.Router()

	req := httptest.NewRequest("POST", "/v1/fetch", bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var errResp ErrorResponse
	err = json.NewDecoder(w.Body).Decode(&errResp)
	require.NoError(t, err)
	assert.Contains(t, errResp.Error, "Invalid JSON")
}

// TestHandleFetchInvalidURL verifies invalid URL in request is rejected.
func TestHandleFetchInvalidURL(t *testing.T) {
	c, err := client.New(nil)
	require.NoError(t, err)
	defer c.Close()

	s, err := New(c, nil, nil)
	require.NoError(t, err)

	router := s.Router()

	reqBody := FetchRequest{
		URL: "not-a-valid-url",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/v1/fetch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestSendJSON verifies JSON response formatting.
func TestSendJSON(t *testing.T) {
	c, err := client.New(nil)
	require.NoError(t, err)
	defer c.Close()

	s, err := New(c, nil, nil)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	data := map[string]string{"key": "value"}

	s.sendJSON(w, data, http.StatusOK)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "application/json")

	var result map[string]string
	err = json.NewDecoder(w.Body).Decode(&result)
	require.NoError(t, err)
	assert.Equal(t, "value", result["key"])
}

// TestSendError verifies error response formatting.
func TestSendError(t *testing.T) {
	c, err := client.New(nil)
	require.NoError(t, err)
	defer c.Close()

	s, err := New(c, nil, nil)
	require.NoError(t, err)

	w := httptest.NewRecorder()

	s.sendError(w, "test error", http.StatusBadRequest)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var errResp ErrorResponse
	err = json.NewDecoder(w.Body).Decode(&errResp)
	require.NoError(t, err)
	assert.Equal(t, "test error", errResp.Error)
	assert.Equal(t, http.StatusBadRequest, errResp.StatusCode)
}

// TestBuildFetchMetadata verifies metadata building.
func TestBuildFetchMetadata(t *testing.T) {
	resp := &client.Response{
		URL:        "https://example.com",
		StatusCode: 200,
		Headers: map[string][]string{
			"Content-Type": {"text/html; charset=utf-8"},
		},
		Title:       "Test Page",
		Description: "Test Description",
		CacheState:  "hit",
	}

	metadata := buildFetchMetadata(resp, "text/html", "en", "Wed, 21 Oct 2015 07:28:00 GMT", 1000)

	assert.Equal(t, "https://example.com", metadata.URL)
	assert.Equal(t, 200, metadata.StatusCode)
	assert.Equal(t, "text/html", metadata.ContentType)
	assert.Equal(t, "en", metadata.Language)
	assert.Equal(t, "Test Page", metadata.Title)
	assert.Equal(t, "Test Description", metadata.Description)
	assert.Equal(t, 1000, metadata.EstimatedTokens)
	assert.Equal(t, "Wed, 21 Oct 2015 07:28:00 GMT", metadata.LastModified)
	assert.Equal(t, "hit", metadata.CacheState)
}

// TestExtractLanguage verifies language extraction from HTML.
func TestExtractLanguage(t *testing.T) {
	tests := []struct {
		html     string
		expected string
	}{
		{`<html lang="en">`, "en"},
		{`<html lang="en-US">`, "en"}, // Intentionally truncates to base language
		{`<HTML LANG="fr">`, "fr"},
		{`<html lang='de'>`, "de"},
		{`<html lang="pt-BR">`, "pt"}, // Truncates to base language
		{`<html>`, ""},
		{`no html tag`, ""},
	}

	for _, tt := range tests {
		result := extractLanguage([]byte(tt.html))
		assert.Equal(t, tt.expected, result, "html: %s", tt.html)
	}
}

// TestServerCreation verifies server can be created.
func TestServerCreation(t *testing.T) {
	c, err := client.New(nil)
	require.NoError(t, err)
	defer c.Close()

	s, err := New(c, nil, nil)

	require.NoError(t, err)
	assert.NotNil(t, s)
}

// TestServerCreationNilClient verifies nil client is rejected.
func TestServerCreationNilClient(t *testing.T) {
	_, err := New(nil, nil, nil)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "client cannot be nil")
}

// TestServerCreationDefaultConfig verifies server uses defaults when config is nil.
func TestServerCreationDefaultConfig(t *testing.T) {
	c, err := client.New(nil)
	require.NoError(t, err)
	defer c.Close()

	s, err := New(c, nil, nil)

	require.NoError(t, err)
	assert.NotNil(t, s)
}

// TestServerRouter verifies router is configured with all routes.
func TestServerRouter(t *testing.T) {
	c, err := client.New(nil)
	require.NoError(t, err)
	defer c.Close()

	s, err := New(c, nil, nil)
	require.NoError(t, err)

	router := s.Router()

	assert.NotNil(t, router)

	// Test that routes are registered
	routes := []struct {
		method string
		path   string
	}{
		{"GET", "/health"},
		{"POST", "/v1/fetch"},
	}

	for _, route := range routes {
		req := httptest.NewRequest(route.method, route.path, nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		// Should not return 404 (route exists)
		// May return other errors (auth, validation, etc) but route is registered
		assert.NotEqual(t, http.StatusNotFound, w.Code, "route %s %s should exist", route.method, route.path)
	}
}
