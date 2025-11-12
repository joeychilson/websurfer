package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestAuthMiddleware_NoAPIKey(t *testing.T) {
	originalKey := os.Getenv("API_KEY")
	os.Unsetenv("API_KEY")
	defer func() {
		if originalKey != "" {
			os.Setenv("API_KEY", originalKey)
		}
	}()

	middleware := AuthMiddleware()

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestAuthMiddleware_ValidKey(t *testing.T) {
	testKey := "test-secret-key-12345"
	os.Setenv("API_KEY", testKey)
	defer os.Unsetenv("API_KEY")

	middleware := AuthMiddleware()

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	}))

	tests := []struct {
		name       string
		headerName string
		headerVal  string
	}{
		{
			name:       "X-API-Key header",
			headerName: "X-API-Key",
			headerVal:  testKey,
		},
		{
			name:       "Authorization Bearer",
			headerName: "Authorization",
			headerVal:  "Bearer " + testKey,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.Header.Set(tt.headerName, tt.headerVal)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("expected status 200, got %d", w.Code)
			}
		})
	}
}

func TestAuthMiddleware_InvalidKey(t *testing.T) {
	testKey := "test-secret-key-12345"
	os.Setenv("API_KEY", testKey)
	defer os.Unsetenv("API_KEY")

	middleware := AuthMiddleware()

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	}))

	tests := []struct {
		name       string
		headerName string
		headerVal  string
		wantCode   int
	}{
		{
			name:       "wrong key",
			headerName: "X-API-Key",
			headerVal:  "wrong-key",
			wantCode:   http.StatusUnauthorized,
		},
		{
			name:       "similar key (timing attack test)",
			headerName: "X-API-Key",
			headerVal:  "test-secret-key-12344",
			wantCode:   http.StatusUnauthorized,
		},
		{
			name:       "prefix match (timing attack test)",
			headerName: "X-API-Key",
			headerVal:  "test-secret-key",
			wantCode:   http.StatusUnauthorized,
		},
		{
			name:       "empty key",
			headerName: "X-API-Key",
			headerVal:  "",
			wantCode:   http.StatusUnauthorized,
		},
		{
			name:     "no key header",
			wantCode: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			if tt.headerName != "" {
				req.Header.Set(tt.headerName, tt.headerVal)
			}
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			if w.Code != tt.wantCode {
				t.Errorf("expected status %d, got %d", tt.wantCode, w.Code)
			}
		})
	}
}

func TestAuthMiddleware_ConstantTimeComparison(t *testing.T) {
	testKey := "test-secret-key-12345"
	os.Setenv("API_KEY", testKey)
	defer os.Unsetenv("API_KEY")

	middleware := AuthMiddleware()

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	testKeys := []string{
		"",
		"a",
		"short",
		"test-secret-key-12345",
		"test-secret-key-123456",
		"test-secret-key-1234",
		"very-long-key-that-is-much-longer-than-the-actual-key",
	}

	for _, key := range testKeys {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("X-API-Key", key)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if key == testKey {
			if w.Code != http.StatusOK {
				t.Errorf("expected status 200 for correct key, got %d", w.Code)
			}
		} else {
			if w.Code != http.StatusUnauthorized {
				t.Errorf("expected status 401 for key %q, got %d", key, w.Code)
			}
		}
	}
}
