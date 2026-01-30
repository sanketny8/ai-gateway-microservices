package ratelimit

import (
	"sync"
	"time"
)

// TokenBucket implements token bucket rate limiting algorithm
type TokenBucket struct {
	capacity   int64
	tokens     int64
	refillRate float64
	lastRefill time.Time
	mu         sync.Mutex
}

// NewTokenBucket creates a new token bucket rate limiter
//
// capacity: Maximum number of tokens
// refillRate: Tokens added per second
func NewTokenBucket(capacity int64, refillRate float64) *TokenBucket {
	return &TokenBucket{
		capacity:   capacity,
		tokens:     capacity,
		refillRate: refillRate,
		lastRefill: time.Now(),
	}
}

// Allow checks if the request is allowed and consumes tokens
//
// tokens: Number of tokens to consume
// Returns true if allowed, false if rate limit exceeded
func (tb *TokenBucket) Allow(tokens int64) bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	// Refill tokens based on elapsed time
	tb.refill()

	// Check if enough tokens available
	if tb.tokens >= tokens {
		tb.tokens -= tokens
		return true
	}

	return false
}

// refill adds tokens based on elapsed time
func (tb *TokenBucket) refill() {
	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()

	// Calculate tokens to add
	tokensToAdd := int64(elapsed * tb.refillRate)

	if tokensToAdd > 0 {
		tb.tokens += tokensToAdd
		if tb.tokens > tb.capacity {
			tb.tokens = tb.capacity
		}
		tb.lastRefill = now
	}
}

// Available returns the number of tokens currently available
func (tb *TokenBucket) Available() int64 {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.refill()
	return tb.tokens
}

// RateLimiter manages rate limits for multiple users
type RateLimiter struct {
	buckets map[string]*TokenBucket
	mu      sync.RWMutex

	// Default limits
	defaultCapacity   int64
	defaultRefillRate float64
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(capacity int64, refillRate float64) *RateLimiter {
	return &RateLimiter{
		buckets:           make(map[string]*TokenBucket),
		defaultCapacity:   capacity,
		defaultRefillRate: refillRate,
	}
}

// Allow checks if request from user is allowed
func (rl *RateLimiter) Allow(userID string, tokens int64) bool {
	bucket := rl.getBucket(userID)
	return bucket.Allow(tokens)
}

// getBucket gets or creates a bucket for a user
func (rl *RateLimiter) getBucket(userID string) *TokenBucket {
	rl.mu.RLock()
	bucket, exists := rl.buckets[userID]
	rl.mu.RUnlock()

	if exists {
		return bucket
	}

	// Create new bucket
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Double-check after acquiring write lock
	if bucket, exists := rl.buckets[userID]; exists {
		return bucket
	}

	bucket = NewTokenBucket(rl.defaultCapacity, rl.defaultRefillRate)
	rl.buckets[userID] = bucket
	return bucket
}

// Stats returns stats for a user
func (rl *RateLimiter) Stats(userID string) map[string]interface{} {
	bucket := rl.getBucket(userID)
	return map[string]interface{}{
		"available": bucket.Available(),
		"capacity":  bucket.capacity,
	}
}

