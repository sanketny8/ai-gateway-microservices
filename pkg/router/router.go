package router

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sanketny8/ai-gateway-microservices/pkg/cache"
	"github.com/sanketny8/ai-gateway-microservices/pkg/providers"
	"github.com/sanketny8/ai-gateway-microservices/pkg/ratelimit"
)

// Router handles routing requests to appropriate providers
type Router struct {
	providers   map[string]providers.Provider
	cache       *cache.RedisCache
	rateLimiter *ratelimit.RateLimiter
}

// NewRouter creates a new router
func NewRouter(cache *cache.RedisCache, rateLimiter *ratelimit.RateLimiter) *Router {
	return &Router{
		providers:   make(map[string]providers.Provider),
		cache:       cache,
		rateLimiter: rateLimiter,
	}
}

// RegisterProvider registers a provider
func (r *Router) RegisterProvider(name string, provider providers.Provider) {
	r.providers[name] = provider
}

// HandleChatCompletion handles chat completion requests
func (r *Router) HandleChatCompletion(c *gin.Context) {
	// Extract user ID from header or auth token
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing user ID"})
		return
	}

	// Rate limiting
	if !r.rateLimiter.Allow(userID, 1) {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded"})
		return
	}

	// Parse request
	var req providers.ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Determine provider from model name
	providerName := r.getProviderFromModel(req.Model)
	provider, ok := r.providers[providerName]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported model: " + req.Model})
		return
	}

	// Check cache (only for non-streaming requests)
	if !req.Stream {
		cacheKey := r.generateCacheKey(&req)
		var cachedResp providers.ChatResponse
		if err := r.cache.Get(c.Request.Context(), cacheKey, &cachedResp); err == nil {
			// Cache hit
			c.JSON(http.StatusOK, cachedResp)
			return
		}
	}

	// Call provider
	resp, err := provider.ChatCompletion(&req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Cache response (only for non-streaming)
	if !req.Stream {
		cacheKey := r.generateCacheKey(&req)
		_ = r.cache.Set(c.Request.Context(), cacheKey, resp)
	}

	c.JSON(http.StatusOK, resp)
}

// getProviderFromModel determines the provider from the model name
func (r *Router) getProviderFromModel(model string) string {
	if strings.HasPrefix(model, "gpt-") {
		return "openai"
	}
	if strings.HasPrefix(model, "claude-") {
		return "anthropic"
	}
	// Add more providers as needed
	return "openai" // default
}

// generateCacheKey generates a cache key from the request
func (r *Router) generateCacheKey(req *providers.ChatRequest) string {
	// Create a deterministic string from the request
	data, _ := json.Marshal(req)
	hash := sha256.Sum256(data)
	return fmt.Sprintf("chat:%s", hex.EncodeToString(hash[:]))
}

