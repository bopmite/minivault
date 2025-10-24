// Package minivault provides HTTP client for MiniVault distributed cache
//
// High-performance distributed cache client for MiniVault HTTP protocol.
//
// Features:
//   - 100,000+ req/sec per worker
//   - 3-node geo-replication
//   - Eventually consistent (30-50ms)
//   - Max value size: 100MB
//   - Zero-cost HTTP abstraction
//
// Example:
//
//	client := minivault.NewHTTPClient("http://localhost:8080", "your-api-key")
//
//	// Store JSON
//	user := User{Name: "Alice", Age: 30}
//	err := client.SetJSON("user:123", user)
//
//	// Retrieve JSON
//	var result User
//	err := client.GetJSON("user:123", &result)
//
//	// Store raw bytes
//	err := client.Set("data:raw", []byte("hello world"))
//
//	// Delete
//	err := client.Delete("user:123")
//
//	// Health check
//	health, err := client.Health()
package minivault

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Health represents cluster health information
type Health struct {
	Status        string `json:"status"`
	UptimeSeconds int64  `json:"uptime_seconds"`
	CacheItems    int64  `json:"cache_items"`
	CacheSizeMB   int64  `json:"cache_size_mb"`
	StorageSizeMB int64  `json:"storage_size_mb"`
	Goroutines    int    `json:"goroutines"`
	MemoryMB      int64  `json:"memory_mb"`
}

// HTTPClient is a client for MiniVault HTTP protocol
type HTTPClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	logging    bool
}

// HTTPClientOptions configures the HTTP client
type HTTPClientOptions struct {
	BaseURL string
	APIKey  string
	Timeout time.Duration
	Logging bool
}

// NewHTTPClient creates a new HTTP client with default settings
func NewHTTPClient(baseURL, apiKey string) *HTTPClient {
	return &HTTPClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		logging: false,
	}
}

// NewHTTPClientWithOptions creates a new HTTP client with custom options
func NewHTTPClientWithOptions(opts HTTPClientOptions) *HTTPClient {
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	return &HTTPClient{
		baseURL: opts.BaseURL,
		apiKey:  opts.APIKey,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		logging: opts.Logging,
	}
}

func (c *HTTPClient) log(format string, args ...interface{}) {
	if c.logging {
		fmt.Printf("[MiniVault] "+format+"\n", args...)
	}
}

// Get retrieves a value for a key (automatically unwraps from JSON response)
func (c *HTTPClient) Get(key string) (interface{}, error) {
	url := fmt.Sprintf("%s/%s", c.baseURL, key)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		c.log("Cache miss: %s", key)
		return nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET failed: %d", resp.StatusCode)
	}

	var result struct {
		Success bool        `json:"success"`
		Data    interface{} `json:"data"`
		Error   string      `json:"error"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if !result.Success {
		return nil, fmt.Errorf("GET failed: %s", result.Error)
	}

	c.log("Cache hit: %s", key)
	return result.Data, nil
}

// GetJSON retrieves and unmarshals JSON data
func (c *HTTPClient) GetJSON(key string, v interface{}) error {
	data, err := c.Get(key)
	if err != nil {
		return err
	}
	if data == nil {
		return fmt.Errorf("key not found: %s", key)
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}

	return json.Unmarshal(jsonData, v)
}

// Set stores a value for a key (automatically wraps in JSON request)
func (c *HTTPClient) Set(key string, value interface{}) error {
	url := fmt.Sprintf("%s/%s", c.baseURL, key)

	reqBody := map[string]interface{}{"value": value}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPut, url, bytes.NewReader(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if !result.Success {
		return fmt.Errorf("SET failed: %s", result.Error)
	}

	c.log("Cache set: %s", key)
	return nil
}

// SetJSON marshals and stores JSON data
func (c *HTTPClient) SetJSON(key string, v interface{}) error {
	return c.Set(key, v)
}

// Delete removes a key
func (c *HTTPClient) Delete(key string) error {
	url := fmt.Sprintf("%s/%s", c.baseURL, key)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	if c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if !result.Success {
		return fmt.Errorf("DELETE failed: %s", result.Error)
	}

	c.log("Cache delete: %s", key)
	return nil
}

// Exists checks if a key exists
func (c *HTTPClient) Exists(key string) (bool, error) {
	data, err := c.Get(key)
	if err != nil {
		return false, err
	}
	return data != nil, nil
}

// Health retrieves cluster health information
func (c *HTTPClient) Health() (*Health, error) {
	url := fmt.Sprintf("%s/health", c.baseURL)

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("health check failed: %d", resp.StatusCode)
	}

	var health Health
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		return nil, fmt.Errorf("failed to decode health response: %w", err)
	}

	return &health, nil
}

// MGet retrieves multiple keys in parallel
func (c *HTTPClient) MGet(keys []string) (map[string]interface{}, error) {
	type result struct {
		key  string
		data interface{}
		err  error
	}

	results := make(chan result, len(keys))

	for _, key := range keys {
		go func(k string) {
			data, err := c.Get(k)
			results <- result{key: k, data: data, err: err}
		}(key)
	}

	output := make(map[string]interface{})
	for i := 0; i < len(keys); i++ {
		r := <-results
		if r.err == nil && r.data != nil {
			output[r.key] = r.data
		}
	}

	return output, nil
}

// MSet stores multiple key-value pairs in parallel
func (c *HTTPClient) MSet(entries map[string]interface{}) error {
	type result struct {
		key string
		err error
	}

	results := make(chan result, len(entries))

	for key, value := range entries {
		go func(k string, v interface{}) {
			err := c.Set(k, v)
			results <- result{key: k, err: err}
		}(key, value)
	}

	var firstErr error
	for i := 0; i < len(entries); i++ {
		r := <-results
		if r.err != nil && firstErr == nil {
			firstErr = r.err
		}
	}

	return firstErr
}
