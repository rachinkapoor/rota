package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/alpkeskin/rota/core/internal/models"
	"github.com/alpkeskin/rota/core/pkg/logger"
)

// WebshareClient handles communication with Webshare API
type WebshareClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	logger     *logger.Logger
}

// NewWebshareClient creates a new WebshareClient
func NewWebshareClient(apiKey string, log *logger.Logger) *WebshareClient {
	return &WebshareClient{
		baseURL: "https://proxy.webshare.io",
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: log,
	}
}

// ListProxies fetches all proxies from Webshare API with pagination
func (c *WebshareClient) ListProxies(ctx context.Context, mode string) ([]models.WebshareProxy, error) {
	var allProxies []models.WebshareProxy
	url := fmt.Sprintf("%s/api/v2/proxy/list/?mode=%s", c.baseURL, mode)

	for url != "" {
		proxies, nextURL, err := c.fetchProxyPage(ctx, url)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch proxy page: %w", err)
		}

		allProxies = append(allProxies, proxies...)
		url = nextURL
	}

	return allProxies, nil
}

// fetchProxyPage fetches a single page of proxies
func (c *WebshareClient) fetchProxyPage(ctx context.Context, url string) ([]models.WebshareProxy, string, error) {
	var response models.WebshareProxyListResponse

	err := c.requestWithRetry(ctx, "GET", url, nil, &response)
	if err != nil {
		return nil, "", err
	}

	nextURL := ""
	if response.Next != nil {
		nextURL = *response.Next
	}

	return response.Results, nextURL, nil
}

// CreateReplacement creates a replacement request for proxies
func (c *WebshareClient) CreateReplacement(ctx context.Context, ips []string) (*models.WebshareReplacementResponse, error) {
	url := fmt.Sprintf("%s/api/v3/proxy/replace/", c.baseURL)

	reqBody := models.WebshareReplacementRequest{
		ToReplace: struct {
			ProxyAddressIn []string `json:"proxy_address__in"`
		}{
			ProxyAddressIn: ips,
		},
		ReplaceWith: []map[string]interface{}{
			{"random": true},
		},
		DryRun: false,
	}

	var response models.WebshareReplacementResponse
	err := c.requestWithRetry(ctx, "POST", url, reqBody, &response)
	if err != nil {
		return nil, err
	}

	return &response, nil
}

// GetReplacementStatus gets the status of a replacement request
func (c *WebshareClient) GetReplacementStatus(ctx context.Context, id int) (*models.WebshareReplacementResponse, error) {
	url := fmt.Sprintf("%s/api/v3/proxy/replace/%d/", c.baseURL, id)

	var response models.WebshareReplacementResponse
	err := c.requestWithRetry(ctx, "GET", url, nil, &response)
	if err != nil {
		return nil, err
	}

	return &response, nil
}

// requestWithRetry performs an HTTP request with exponential backoff for rate limits
func (c *WebshareClient) requestWithRetry(ctx context.Context, method, url string, body interface{}, response interface{}) error {
	maxRetries := 5
	baseDelay := 1 * time.Second

	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewBuffer(jsonData)
	}

	for attempt := 0; attempt < maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Authorization", fmt.Sprintf("Token %s", c.apiKey))
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("request failed: %w", err)
		}
		defer resp.Body.Close()

		// Handle rate limit (429)
		if resp.StatusCode == http.StatusTooManyRequests {
			delay := baseDelay * time.Duration(1<<uint(attempt)) // Exponential backoff
			c.logger.Warn("rate limit hit, retrying", "attempt", attempt+1, "delay", delay)
			time.Sleep(delay)
			continue
		}

		// Handle other errors
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			bodyBytes, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("API error: status %d, body: %s", resp.StatusCode, string(bodyBytes))
		}

		// Parse response
		if response != nil {
			if err := json.NewDecoder(resp.Body).Decode(response); err != nil {
				return fmt.Errorf("failed to decode response: %w", err)
			}
		}

		return nil
	}

	return fmt.Errorf("max retries exceeded for rate limit")
}
