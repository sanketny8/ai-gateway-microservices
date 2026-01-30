package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"

	"github.com/sanketny8/ai-gateway-microservices/pkg/cache"
	"github.com/sanketny8/ai-gateway-microservices/pkg/middleware"
	"github.com/sanketny8/ai-gateway-microservices/pkg/providers"
	"github.com/sanketny8/ai-gateway-microservices/pkg/ratelimit"
	"github.com/sanketny8/ai-gateway-microservices/pkg/router"
)

func main() {
	// Initialize tracing
	tp, err := initTracer()
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			log.Printf("Error shutting down tracer: %v", err)
		}
	}()

	// Initialize cache
	redisCache, err := cache.NewRedisCache(
		getEnv("REDIS_ADDR", "localhost:6379"),
		getEnv("REDIS_PASSWORD", ""),
		0,
		5*time.Minute,
	)
	if err != nil {
		log.Printf("Warning: Redis cache disabled: %v", err)
		redisCache = nil
	}

	// Initialize rate limiter (100 requests per user per minute)
	rateLimiter := ratelimit.NewRateLimiter(100, 100.0/60.0)

	// Initialize router
	gwRouter := router.NewRouter(redisCache, rateLimiter)

	// Register providers
	if openaiKey := os.Getenv("OPENAI_API_KEY"); openaiKey != "" {
		gwRouter.RegisterProvider("openai", providers.NewOpenAIProvider(openaiKey))
		log.Println("✓ OpenAI provider registered")
	}
	if anthropicKey := os.Getenv("ANTHROPIC_API_KEY"); anthropicKey != "" {
		gwRouter.RegisterProvider("anthropic", providers.NewAnthropicProvider(anthropicKey))
		log.Println("✓ Anthropic provider registered")
	}

	// Create Gin router
	ginRouter := gin.Default()

	// Middleware
	ginRouter.Use(gin.Recovery())
	ginRouter.Use(middleware.TracingMiddleware())
	ginRouter.Use(middleware.MetricsMiddleware())

	// Health endpoints
	ginRouter.GET("/health", healthCheck)
	ginRouter.GET("/ready", readinessCheck)

	// Prometheus metrics
	ginRouter.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// API v1 routes
	v1 := ginRouter.Group("/v1")
	{
		v1.POST("/chat/completions", gwRouter.HandleChatCompletion)
		v1.POST("/embeddings", handleEmbeddings)
		v1.GET("/usage", handleUsage)
	}

	// Start server
	srv := &http.Server{
		Addr:           ":8080",
		Handler:        ginRouter,
		ReadTimeout:    60 * time.Second,
		WriteTimeout:   60 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	// Graceful shutdown
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %s\n", err)
		}
	}()

	log.Println("🚀 AI Gateway started on :8080")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}

	if redisCache != nil {
		redisCache.Close()
	}

	log.Println("Server exited")
}

func initTracer() (*sdktrace.TracerProvider, error) {
	exporter, err := jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint("http://localhost:14268/api/traces")))
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String("ai-gateway"),
		)),
	)
	otel.SetTracerProvider(tp)
	return tp, nil
}

func healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "healthy",
		"service": "ai-gateway",
	})
}

func readinessCheck(c *gin.Context) {
	// Check Redis, DB, etc.
	c.JSON(http.StatusOK, gin.H{
		"ready": true,
	})
}

func handleEmbeddings(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"object": "list",
		"data":   []map[string]interface{}{},
	})
}

func handleUsage(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		userID = "anonymous"
	}

	c.JSON(http.StatusOK, gin.H{
		"user_id":     userID,
		"tokens_used": 12345,
		"requests":    100,
		"cost":        5.67,
	})
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
