package provider

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-log/tflogtest"
)

func TestLoggingTransport_SuccessfulRequest(t *testing.T) {
	// Create a test server that returns 200
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("success")); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	}))
	defer srv.Close()

	// Create context with test logger
	ctx := tflogtest.RootLogger(context.Background(), os.Stdout)

	// Create logging transport
	transport := newLoggingTransport(ctx)

	// Make request
	req, err := http.NewRequest("GET", srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

func TestLoggingTransport_ErrorResponse(t *testing.T) {
	// Create a test server that returns 400 with a body
	expectedBody := `{"errors":[{"code":"MANIFEST_INVALID","message":"manifest invalid"}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		if _, err := w.Write([]byte(expectedBody)); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	}))
	defer srv.Close()

	// Create context with test logger at TRACE level
	os.Setenv("TF_LOG", "TRACE")
	defer os.Unsetenv("TF_LOG")
	ctx := tflogtest.RootLogger(context.Background(), os.Stdout)

	// Create logging transport
	transport := newLoggingTransport(ctx)

	// Make request with manifest body
	manifestBody := `{"schemaVersion":2,"config":{"digest":"sha256:abc123"}}`
	req, err := http.NewRequest("PUT", srv.URL+"/v2/test/manifests/latest", strings.NewReader(manifestBody))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")

	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", resp.StatusCode)
	}

	// Verify response body is still readable (not consumed by logging)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}
	if string(body) != expectedBody {
		t.Errorf("expected body %q, got %q", expectedBody, string(body))
	}
}

func TestLoggingTransport_ErrorResponseNonTrace(t *testing.T) {
	// Create a test server that returns 400 with a body
	expectedBody := `{"errors":[{"code":"MANIFEST_INVALID","message":"manifest invalid"}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		if _, err := w.Write([]byte(expectedBody)); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	}))
	defer srv.Close()

	// Create context with test logger at DEBUG level (not TRACE)
	os.Setenv("TF_LOG", "DEBUG")
	defer os.Unsetenv("TF_LOG")
	ctx := tflogtest.RootLogger(context.Background(), os.Stdout)

	// Create logging transport
	transport := newLoggingTransport(ctx)

	// Make request
	req, err := http.NewRequest("GET", srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip failed: %v", err)
	}
	defer resp.Body.Close()

	// At DEBUG level, the response body should still be restored
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}
	if string(body) != expectedBody {
		t.Errorf("expected body %q, got %q", expectedBody, string(body))
	}
}

func TestLoggingTransport_RequestBodyCapture(t *testing.T) {
	// Verify that request body is captured and restored properly
	requestBody := `{"test":"data"}`
	receivedBody := ""

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx := tflogtest.RootLogger(context.Background(), os.Stdout)
	transport := newLoggingTransport(ctx)

	req, err := http.NewRequest("POST", srv.URL, bytes.NewReader([]byte(requestBody)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	_, err = transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip failed: %v", err)
	}

	if receivedBody != requestBody {
		t.Errorf("server received %q, expected %q", receivedBody, requestBody)
	}
}

func TestShouldLogBody(t *testing.T) {
	tests := []struct {
		contentType string
		want        bool
	}{
		{"application/json", true},
		{"application/vnd.oci.image.manifest.v1+json", true},
		{"application/vnd.docker.distribution.manifest.v2+json", true},
		{"application/vnd.oci.image.index.v1+json", true},
		{"text/plain", false},
		{"text/html", false},
		{"application/octet-stream", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.contentType, func(t *testing.T) {
			got := shouldLogBody(tt.contentType)
			if got != tt.want {
				t.Errorf("shouldLogBody(%q) = %v, want %v", tt.contentType, got, tt.want)
			}
		})
	}
}

func TestLoggingTransport_CapturesManifestContentTypes(t *testing.T) {
	// Verify that manifest content types trigger body capture
	manifestContentTypes := []string{
		"application/vnd.oci.image.manifest.v1+json",
		"application/vnd.docker.distribution.manifest.v2+json",
		"application/vnd.docker.distribution.manifest.list.v2+json",
	}

	for _, ct := range manifestContentTypes {
		t.Run(ct, func(t *testing.T) {
			requestBody := `{"schemaVersion":2}`
			capturedRequest := false

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				if string(body) == requestBody {
					capturedRequest = true
				}
				w.WriteHeader(http.StatusBadRequest)
				if _, err := w.Write([]byte(`{"error":"test"}`)); err != nil {
					t.Errorf("failed to write response: %v", err)
				}
			}))
			defer srv.Close()

			os.Setenv("TF_LOG", "TRACE")
			defer os.Unsetenv("TF_LOG")
			ctx := tflogtest.RootLogger(context.Background(), os.Stdout)
			transport := newLoggingTransport(ctx)

			req, _ := http.NewRequest("PUT", srv.URL, strings.NewReader(requestBody))
			req.Header.Set("Content-Type", ct)

			resp, err := transport.RoundTrip(req)
			if err != nil {
				t.Fatalf("RoundTrip failed: %v", err)
			}
			resp.Body.Close()

			if !capturedRequest {
				t.Error("request body was not properly captured and restored")
			}
		})
	}
}

func TestLoggingTransport_PreservesContext(t *testing.T) {
	// Verify that the transport uses the context passed to it
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Create a context with a test value
	type contextKey string
	key := contextKey("test")
	ctx := context.WithValue(context.Background(), key, "value")
	ctx = tflogtest.RootLogger(ctx, os.Stdout)

	transport := newLoggingTransport(ctx)

	// Verify the transport stored the context
	lt := transport.(*loggingTransport)
	if lt.ctx.Value(key) != "value" {
		t.Error("transport did not preserve context")
	}
}

func TestIsExpectedProtocolResponse(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		path     string
		status   int
		expected bool
	}{
		{
			name:     "401 on /v2/ is auth challenge, not error",
			method:   "GET",
			path:     "/v2/",
			status:   401,
			expected: true,
		},
		{
			name:     "401 on manifest path is real error",
			method:   "PUT",
			path:     "/v2/test/manifests/latest",
			status:   401,
			expected: false,
		},
		{
			name:     "404 HEAD on blob is existence check, not error",
			method:   "HEAD",
			path:     "/v2/test/blobs/sha256:abc123",
			status:   404,
			expected: true,
		},
		{
			name:     "404 HEAD on manifest is existence check, not error",
			method:   "HEAD",
			path:     "/v2/test/manifests/sha256:abc123",
			status:   404,
			expected: true,
		},
		{
			name:     "404 GET is real error (not a HEAD check)",
			method:   "GET",
			path:     "/v2/test/blobs/sha256:abc123",
			status:   404,
			expected: false,
		},
		{
			name:     "400 bad request is real error",
			method:   "PUT",
			path:     "/v2/test/manifests/latest",
			status:   400,
			expected: false,
		},
		{
			name:     "500 server error is real error",
			method:   "PUT",
			path:     "/v2/test/manifests/latest",
			status:   500,
			expected: false,
		},
		{
			name:     "403 forbidden is real error",
			method:   "GET",
			path:     "/v2/test/manifests/latest",
			status:   403,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest(tt.method, "https://example.com"+tt.path, nil)
			resp := &http.Response{StatusCode: tt.status}
			got := isExpectedProtocolResponse(req, resp)
			if got != tt.expected {
				t.Errorf("isExpectedProtocolResponse() = %v, want %v", got, tt.expected)
			}
		})
	}
}
