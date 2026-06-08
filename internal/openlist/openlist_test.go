package openlist

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// mockOpenList is a test helper that creates a mock OpenList API server.
type mockOpenList struct {
	server *httptest.Server
	t      *testing.T
}

// newMockServer creates a mock OpenList server with the given handler.
// If handler is nil, a default handler is used that returns empty content.
func newMockServer(t *testing.T, handler http.HandlerFunc) *mockOpenList {
	t.Helper()
	m := &mockOpenList{t: t}
	m.server = httptest.NewServer(handler)
	return m
}

func (m *mockOpenList) URL() string {
	return m.server.URL
}

func (m *mockOpenList) Close() {
	m.server.Close()
}

// defaultListHandler returns a handler that serves a static directory listing.
func defaultListHandler(path string, files []FileInfo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Path string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", 400)
			return
		}

		if req.Path != path {
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"code":    404,
				"message": "path not found",
			})
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    200,
			"message": "success",
			"data": map[string]interface{}{
				"content": files,
				"total":   len(files),
			},
		})
	}
}

func TestNew(t *testing.T) {
	c := New("http://example.com", "pass", 10*time.Second, 3)
	if c.baseURL != "http://example.com" {
		t.Errorf("expected baseURL http://example.com, got %s", c.baseURL)
	}
	if c.password != "pass" {
		t.Errorf("expected password pass, got %s", c.password)
	}
	if c.httpClient.Timeout != 10*time.Second {
		t.Errorf("expected timeout 10s, got %s", c.httpClient.Timeout)
	}
	if c.retryMax != 3 {
		t.Errorf("expected retryMax 3, got %d", c.retryMax)
	}
}

func TestNew_Defaults(t *testing.T) {
	c := New("http://example.com", "", 0, 0)
	if c.httpClient.Timeout != defaultTimeout {
		t.Errorf("expected default timeout, got %s", c.httpClient.Timeout)
	}
	if c.retryMax != defaultRetryMax {
		t.Errorf("expected default retryMax, got %d", c.retryMax)
	}
}

func TestList_Success(t *testing.T) {
	expected := []FileInfo{
		{Name: "movie.mp4", Path: "/movies/movie.mp4", Size: 1024, IsDir: false, Modified: "2024-01-01T00:00:00Z"},
		{Name: "tv", Path: "/movies/tv", Size: 0, IsDir: true, Modified: "2024-01-01T00:00:00Z"},
	}

	handler := defaultListHandler("/movies", expected)
	mock := newMockServer(t, handler)
	defer mock.Close()

	client := New(mock.URL(), "", 0, 0)
	result, err := client.List(context.Background(), "/movies")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if result.Total != 2 {
		t.Errorf("expected total 2, got %d", result.Total)
	}
	if len(result.Content) != 2 {
		t.Errorf("expected 2 files, got %d", len(result.Content))
	}
	if result.Content[0].Name != "movie.mp4" {
		t.Errorf("expected first file name movie.mp4, got %s", result.Content[0].Name)
	}
	if result.Content[1].IsDir != true {
		t.Errorf("expected second entry to be a directory")
	}
}

func TestGet_Success(t *testing.T) {
	mock := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Path string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", 400)
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    200,
			"message": "success",
			"data": map[string]interface{}{
				"name":     "movie.mp4",
				"path":     req.Path,
				"size":     2048,
				"is_dir":   false,
				"modified": "2024-01-01T00:00:00Z",
			},
		})
	})
	defer mock.Close()

	client := New(mock.URL(), "", 0, 0)
	info, err := client.Get(context.Background(), "/movies/movie.mp4")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if info.Name != "movie.mp4" {
		t.Errorf("expected name movie.mp4, got %s", info.Name)
	}
	if info.Path != "/movies/movie.mp4" {
		t.Errorf("expected path /movies/movie.mp4, got %s", info.Path)
	}
	if info.Size != 2048 {
		t.Errorf("expected size 2048, got %d", info.Size)
	}
	if info.IsDir != false {
		t.Errorf("expected is_dir false, got %v", info.IsDir)
	}
}

func TestDelete_Success(t *testing.T) {
	mock := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Path string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", 400)
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    200,
			"message": "success",
		})
	})
	defer mock.Close()

	client := New(mock.URL(), "", 0, 0)
	err := client.Delete(context.Background(), "/movies/movie.mp4")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
}

func TestList_NotFound(t *testing.T) {
	mock := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    404,
			"message": "path not found",
		})
	})
	defer mock.Close()

	client := New(mock.URL(), "", 0, 0)
	_, err := client.List(context.Background(), "/nonexistent")
	if err == nil {
		t.Fatal("expected error for not found")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected not found error, got: %v", err)
	}
}

func TestList_Unauthorized(t *testing.T) {
	mock := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    401,
			"message": "unauthorized",
		})
	})
	defer mock.Close()

	client := New(mock.URL(), "wrong-password", 0, 0)
	_, err := client.List(context.Background(), "/movies")
	if err == nil {
		t.Fatal("expected error for unauthorized")
	}
	if !strings.Contains(err.Error(), "authentication") {
		t.Errorf("expected auth error, got: %v", err)
	}
}

func TestList_Forbidden(t *testing.T) {
	mock := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    403,
			"message": "forbidden",
		})
	})
	defer mock.Close()

	client := New(mock.URL(), "", 0, 0)
	_, err := client.List(context.Background(), "/restricted")
	if err == nil {
		t.Fatal("expected error for forbidden")
	}
	if !strings.Contains(err.Error(), "forbidden") {
		t.Errorf("expected forbidden error, got: %v", err)
	}
}

func TestList_RateLimit(t *testing.T) {
	attempts := 0
	mock := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    429,
			"message": "too many requests",
		})
	})
	defer mock.Close()

	client := New(mock.URL(), "", 0, 2) // 2 retries = 3 total attempts
	_, err := client.List(context.Background(), "/movies")
	if err == nil {
		t.Fatal("expected error after rate limit retries exhausted")
	}
	if !strings.Contains(err.Error(), "rate limited") {
		t.Errorf("expected rate limited error, got: %v", err)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts (1 initial + 2 retries), got %d", attempts)
	}
}

func TestList_ServerError_Retry(t *testing.T) {
	attempts := 0
	mock := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"code":    500,
				"message": "internal error",
			})
			return
		}
		// Succeed on 3rd attempt
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code": 200,
			"data": map[string]interface{}{
				"content": []interface{}{},
				"total":   0,
			},
		})
	})
	defer mock.Close()

	client := New(mock.URL(), "", 0, 5)
	_, err := client.List(context.Background(), "/movies")
	if err != nil {
		t.Fatalf("expected success after retry, got: %v", err)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts before success, got %d", attempts)
	}
}

func TestList_ContextCancelled(t *testing.T) {
	mock := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code": 200,
			"data": map[string]interface{}{
				"content": []interface{}{},
				"total":   0,
			},
		})
	})
	defer mock.Close()

	client := New(mock.URL(), "", 0, 0)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	time.Sleep(5 * time.Millisecond) // Let the timeout expire
	_, err := client.List(ctx, "/movies")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestGet_APIError(t *testing.T) {
	mock := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    500,
			"message": "internal server error",
		})
	})
	defer mock.Close()

	client := New(mock.URL(), "", 0, 0)
	_, err := client.Get(context.Background(), "/movies/file.mp4")
	if err == nil {
		t.Fatal("expected API error")
	}
	var apiErr *APIError
	if !isAPIError(err, &apiErr) {
		t.Fatalf("expected APIError, got %T", err)
	}
	if apiErr.Code != 500 {
		t.Errorf("expected code 500, got %d", apiErr.Code)
	}
	if apiErr.Message != "internal server error" {
		t.Errorf("expected message 'internal server error', got %s", apiErr.Message)
	}
}

func TestAuthHeader(t *testing.T) {
	var authHeader string
	mock := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code": 200,
			"data": map[string]interface{}{
				"content": []interface{}{},
				"total":   0,
			},
		})
	})
	defer mock.Close()

	client := New(mock.URL(), "my-token", 0, 0)
	_, err := client.List(context.Background(), "/movies")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if authHeader != "my-token" {
		t.Errorf("expected Authorization header 'my-token', got '%s'", authHeader)
	}
}

func isAPIError(err error, target **APIError) bool {
	for e := err; e != nil; e = unwrap(e) {
		if ae, ok := e.(*APIError); ok {
			*target = ae
			return true
		}
	}
	return false
}

func unwrap(err error) error {
	type unwrapper interface {
		Unwrap() error
	}
	u, ok := err.(unwrapper)
	if !ok {
		return nil
	}
	return u.Unwrap()
}

func TestDelete_NonRetryableError(t *testing.T) {
	mock := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    404,
			"message": "not found",
		})
	})
	defer mock.Close()

	client := New(mock.URL(), "", 0, 3)
	err := client.Delete(context.Background(), "/nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected not found error, got: %v", err)
	}
}

func TestContentTypeHeader(t *testing.T) {
	var contentType string
	mock := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		contentType = r.Header.Get("Content-Type")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code": 200,
			"data": map[string]interface{}{
				"content": []interface{}{},
				"total":   0,
			},
		})
	})
	defer mock.Close()

	client := New(mock.URL(), "", 0, 0)
	_, err := client.List(context.Background(), "/movies")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if contentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", contentType)
	}
}

func TestList_Pagination(t *testing.T) {
	pageCalls := 0
	mock := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Page int `json:"page"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", 400)
			return
		}
		pageCalls++

		// Return one file per page, total=3
		files := []FileInfo{
			{Name: fmt.Sprintf("file%d.mp4", req.Page), Path: fmt.Sprintf("/files/file%d.mp4", req.Page), Size: 100, IsDir: false},
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code": 200,
			"data": map[string]interface{}{
				"content": files,
				"total":   3,
			},
		})
	})
	defer mock.Close()

	client := New(mock.URL(), "", 0, 0)
	result, err := client.List(context.Background(), "/files")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if pageCalls != 3 {
		t.Errorf("expected 3 page calls, got %d", pageCalls)
	}
	if len(result.Content) != 3 {
		t.Errorf("expected 3 files total, got %d", len(result.Content))
	}
}

func TestErrorClassification(t *testing.T) {
	tests := []struct {
		statusCode int
		expectErr  error
		retryable  bool
	}{
		{401, ErrAuth, false},
		{403, ErrForbidden, false},
		{404, ErrNotFound, false},
		{429, ErrRateLimit, true},
		{500, ErrServer, true},
		{502, ErrServer, true},
		{503, ErrServer, true},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("HTTP_%d", tt.statusCode), func(t *testing.T) {
			err := classifyHTTPError(tt.statusCode, []byte("error"))
			if !strings.Contains(err.Error(), tt.expectErr.Error()) {
				t.Errorf("expected %v, got %v", tt.expectErr, err)
			}
			if isRetryable(err) != tt.retryable {
				t.Errorf("expected retryable=%v, got %v", tt.retryable, isRetryable(err))
			}
		})
	}
}

func TestBackoffDuration(t *testing.T) {
	prev := time.Duration(0)
	for i := 0; i < 5; i++ {
		d := backoffDuration(i)
		if d <= 0 {
			t.Errorf("expected positive duration for attempt %d", i)
		}
		if d < prev {
			t.Errorf("expected backoff to grow, attempt %d: %v < %v", i, d, prev)
		}
		if d > maxRetryWait {
			t.Errorf("expected backoff capped at %v, got %v", maxRetryWait, d)
		}
		prev = d
	}
}
