package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/sanketny8/ai-gateway-microservices/pkg/providers"
	"github.com/sanketny8/ai-gateway-microservices/pkg/ratelimit"
	"github.com/sanketny8/ai-gateway-microservices/pkg/router"
)

// MockProvider is a mock LLM provider for testing
type MockProvider struct{}

func (m *MockProvider) Name() string {
	return "mock"
}

func (m *MockProvider) ChatCompletion(req *providers.ChatRequest) (*providers.ChatResponse, error) {
	return &providers.ChatResponse{
		ID:      "mock-123",
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: []providers.Choice{
			{
				Index: 0,
				Message: providers.Message{
					Role:    "assistant",
					Content: "This is a mock response",
				},
				FinishReason: "stop",
			},
		},
		Usage: providers.Usage{
			PromptTokens:     10,
			CompletionTokens: 20,
			TotalTokens:      30,
		},
	}, nil
}

func setupTestRouter() *router.Router {
	rateLimiter := ratelimit.NewRateLimiter(100, 1.0)
	r := router.NewRouter(nil, rateLimiter) // nil cache for testing
	r.RegisterProvider("mock", &MockProvider{})
	return r
}

func TestChatCompletion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := setupTestRouter()

	// Create test router
	ginRouter := gin.New()
	ginRouter.POST("/v1/chat/completions", r.HandleChatCompletion)

	// Create test request
	reqBody := providers.ChatRequest{
		Model: "mock-model",
		Messages: []providers.Message{
			{Role: "user", Content: "Hello"},
		},
	}
	body, _ := json.Marshal(reqBody)

	req, _ := http.NewRequest("POST", "/v1/chat/completions", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", "test-user")

	// Record response
	w := httptest.NewRecorder()
	ginRouter.ServeHTTP(w, req)

	// Assert response
	assert.Equal(t, http.StatusOK, w.Code)

	var resp providers.ChatResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Equal(t, "mock-123", resp.ID)
	assert.Len(t, resp.Choices, 1)
	assert.Equal(t, "This is a mock response", resp.Choices[0].Message.Content)
}

func TestRateLimiting(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rateLimiter := ratelimit.NewRateLimiter(2, 0.1) // 2 requests capacity, slow refill
	r := router.NewRouter(nil, rateLimiter)
	r.RegisterProvider("mock", &MockProvider{})

	ginRouter := gin.New()
	ginRouter.POST("/v1/chat/completions", r.HandleChatCompletion)

	reqBody := providers.ChatRequest{
		Model:    "mock-model",
		Messages: []providers.Message{{Role: "user", Content: "Hello"}},
	}
	body, _ := json.Marshal(reqBody)

	// First request - should succeed
	req1, _ := http.NewRequest("POST", "/v1/chat/completions", bytes.NewBuffer(body))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("X-User-ID", "test-user")
	w1 := httptest.NewRecorder()
	ginRouter.ServeHTTP(w1, req1)
	assert.Equal(t, http.StatusOK, w1.Code)

	// Second request - should succeed
	req2, _ := http.NewRequest("POST", "/v1/chat/completions", bytes.NewBuffer(body))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-User-ID", "test-user")
	w2 := httptest.NewRecorder()
	ginRouter.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code)

	// Third request - should be rate limited
	req3, _ := http.NewRequest("POST", "/v1/chat/completions", bytes.NewBuffer(body))
	req3.Header.Set("Content-Type", "application/json")
	req3.Header.Set("X-User-ID", "test-user")
	w3 := httptest.NewRecorder()
	ginRouter.ServeHTTP(w3, req3)
	assert.Equal(t, http.StatusTooManyRequests, w3.Code)
}

