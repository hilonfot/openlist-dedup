package openlist

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"net/http"
	"sync"
	"time"
)

const (
	defaultTimeout   = 30 * time.Second
	defaultRetryMax  = 3
	baseRetryWait    = 1 * time.Second
	maxRetryWait     = 30 * time.Second
)

// Client is an HTTP client for the OpenList API.
type Client struct {
	baseURL    string
	username   string
	password   string
	token      string
	httpClient *http.Client
	retryMax   int
	mu         sync.Mutex // protects token field during refresh
}

// New creates a new OpenList client.
func New(baseURL, password string, timeout time.Duration, retryMax int) *Client {
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	if retryMax <= 0 {
		retryMax = defaultRetryMax
	}

	return &Client{
		baseURL:  baseURL,
		password: password,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		retryMax: retryMax,
	}
}

// SetUsername sets the username for login.
func (c *Client) SetUsername(username string) {
	c.username = username
}

// loginResponse is the JSON data from /api/auth/login (inside the "data" field).
type loginResponse struct {
	Token string `json:"token"`
}

// Login authenticates with the OpenList API using username and password,
// and stores the returned token for subsequent requests.
func (c *Client) Login(ctx context.Context) error {
	reqBody := map[string]string{
		"username": c.username,
		"password": c.password,
	}

	var resp loginResponse
	if err := c.doRequest(ctx, "/api/auth/login", reqBody, &resp); err != nil {
		return fmt.Errorf("login: %w", err)
	}

	if resp.Token == "" {
		return fmt.Errorf("login: empty token in response")
	}

	c.mu.Lock()
	c.token = resp.Token
	c.mu.Unlock()
	return nil
}

// request sends an authenticated POST request to the given path, retrying on
// retryable errors with exponential backoff. If the token is expired (401), it
// attempts a single re-login and retry.
func (c *Client) request(ctx context.Context, apiPath string, reqBody, respBody interface{}) error {
	var lastErr error

	for attempt := 0; attempt <= c.retryMax; attempt++ {
		lastErr = c.doRequest(ctx, apiPath, reqBody, respBody)
		if lastErr == nil {
			return nil
		}

		// On 401 with token-based auth, try re-login once
		if errors.Is(lastErr, ErrAuth) && c.username != "" {
			if reloginErr := c.reLogin(ctx); reloginErr == nil {
				// Retry the original request with the new token
				lastErr = c.doRequest(ctx, apiPath, reqBody, respBody)
				if lastErr == nil {
					return nil
				}
			}
			// Re-login failed or retry still failed; don't keep trying
			return lastErr
		}

		// Only retry on retryable errors
		if !isRetryable(lastErr) {
			return lastErr
		}

		// Don't sleep on the last attempt
		if attempt < c.retryMax {
			wait := backoffDuration(attempt)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
			}
		}
	}

	return lastErr
}

// reLogin attempts a fresh login and stores the new token. It serializes
// concurrent callers to avoid thundering-herd logins.
func (c *Client) reLogin(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check: another goroutine may have already refreshed
	reqBody := map[string]string{
		"username": c.username,
		"password": c.password,
	}

	var resp loginResponse
	// Use doRequest directly — no retry loop, no re-login recursion
	if err := c.doRequest(ctx, "/api/auth/login", reqBody, &resp); err != nil {
		return err
	}

	if resp.Token == "" {
		return fmt.Errorf("re-login: empty token in response")
	}

	c.token = resp.Token
	return nil
}

// doRequest performs a single HTTP request.
func (c *Client) doRequest(ctx context.Context, apiPath string, reqBody, respBody interface{}) error {
	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("%w: marshal request: %w", ErrEncode, err)
	}

	url := c.baseURL + apiPath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	c.mu.Lock()
	token := c.token
	c.mu.Unlock()
	if token != "" {
		req.Header.Set("Authorization", token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return classifyNetworkError(err)
	}
	defer resp.Body.Close()

	return c.handleResponse(resp, respBody)
}

// handleResponse reads and classifies the HTTP response.
func (c *Client) handleResponse(resp *http.Response, respBody interface{}) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("%w: read response: %w", ErrDecode, err)
	}

	if resp.StatusCode != http.StatusOK {
		return classifyHTTPError(resp.StatusCode, body)
	}

	// Unmarshal into a generic wrapper to check the API-level code
	var apiResp struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return fmt.Errorf("%w: parse api response: %w", ErrDecode, err)
	}

	if apiResp.Code != 200 {
		return &APIError{
			Code:    apiResp.Code,
			Message: apiResp.Message,
		}
	}

	// If the caller wants the data, unmarshal it
	if respBody != nil && apiResp.Data != nil {
		if err := json.Unmarshal(apiResp.Data, respBody); err != nil {
			return fmt.Errorf("%w: parse response data: %w", ErrDecode, err)
		}
	} else if respBody != nil {
		// No data field, try whole body
		if err := json.Unmarshal(body, respBody); err != nil {
			return fmt.Errorf("%w: parse full response: %w", ErrDecode, err)
		}
	}

	return nil
}

// backoffDuration returns the wait time for the given retry attempt using
// exponential backoff with jitter.
func backoffDuration(attempt int) time.Duration {
	wait := float64(baseRetryWait) * math.Pow(2, float64(attempt))
	if wait > float64(maxRetryWait) {
		wait = float64(maxRetryWait)
	}
	// Add ±25% jitter. math/rand/v2's top-level source is concurrency-safe and
	// well-distributed, so parallel requests don't back off in lockstep the way
	// the old time.Now().UnixNano()%500 source could.
	jitter := 0.75 + rand.Float64()*0.5
	return time.Duration(wait * jitter)
}
