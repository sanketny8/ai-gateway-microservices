# AI Gateway Architecture

## System Overview

The AI Gateway is a production-ready microservice that acts as a unified interface for multiple LLM providers (OpenAI, Anthropic, etc.). It provides caching, rate limiting, observability, and cost optimization.

## Core Components

### 1. **API Gateway (main.go)**
- **Framework**: Gin Web Framework
- **Responsibilities**:
  - HTTP request handling
  - Middleware orchestration
  - Graceful shutdown
  - Health checks
- **Endpoints**:
  - `POST /v1/chat/completions` - Chat completions
  - `POST /v1/embeddings` - Text embeddings
  - `GET /v1/usage` - Usage statistics
  - `GET /health` - Liveness probe
  - `GET /ready` - Readiness probe
  - `GET /metrics` - Prometheus metrics

### 2. **Provider Abstraction (pkg/providers/)**
- **Interface**: `Provider` interface for all LLM providers
- **Implementations**:
  - `OpenAIProvider` - OpenAI (GPT-4, GPT-3.5)
  - `AnthropicProvider` - Anthropic (Claude 3)
- **Features**:
  - Unified request/response format
  - HTTP client with timeouts
  - Error handling & retries
  - Response normalization

### 3. **Router (pkg/router/)**
- **Responsibilities**:
  - Model-to-provider mapping
  - Cache lookup (pre-request)
  - Rate limit enforcement
  - Provider invocation
  - Cache storage (post-response)
- **Logic**:
  ```
  Request → Check Rate Limit → Check Cache → Route to Provider → Store in Cache → Response
  ```

### 4. **Rate Limiter (pkg/ratelimit/)**
- **Algorithm**: Token Bucket
- **Features**:
  - Per-user rate limiting
  - Configurable capacity & refill rate
  - Thread-safe with mutex
  - Automatic token refill
- **Default**: 100 requests/minute per user

### 5. **Cache (pkg/cache/)**
- **Backend**: Redis
- **Features**:
  - Semantic caching (request hash-based)
  - Configurable TTL (default: 5 minutes)
  - JSON serialization
  - Connection pooling
- **Key Format**: `chat:<sha256-hash-of-request>`

### 6. **Middleware (pkg/middleware/)**

#### a) **Tracing Middleware**
- **Tool**: OpenTelemetry + Jaeger
- **Data Collected**:
  - HTTP method, URL, status code
  - Request duration
  - User agent
- **Span Attributes**: `http.method`, `http.url`, `http.status_code`

#### b) **Metrics Middleware**
- **Tool**: Prometheus
- **Metrics**:
  - `http_requests_total{method, endpoint, status}`
  - `http_request_duration_seconds{method, endpoint}`
  - `llm_requests_total{provider, model, status}`
  - `llm_request_duration_seconds{provider, model}`
  - `llm_tokens_used_total{provider, model, type}`
  - `cache_hits_total`
  - `cache_misses_total`
  - `rate_limit_exceeded_total{user_id}`

#### c) **Logging Middleware**
- **Tool**: Zap (structured logging)
- **Data Logged**:
  - Timestamp (ISO 8601)
  - HTTP method, path, query
  - Status code
  - Duration
  - Client IP, user agent
  - Errors (if any)

#### d) **Auth Middleware**
- **Method**: Bearer token (API keys)
- **Flow**:
  1. Extract `Authorization: Bearer <token>`
  2. Validate against valid keys map
  3. Store `user_id` in context
  4. Reject if invalid (401)

## Data Flow

### Request Flow (Chat Completion)

```
┌──────────┐
│  Client  │
└────┬─────┘
     │ POST /v1/chat/completions
     │ Headers: X-User-ID: user-123
     │ Body: {model: "gpt-4", messages: [...]}
     ↓
┌─────────────────────────────────────┐
│   Gin Router (main.go)              │
│   • Parse request                   │
│   • Apply middleware stack:         │
│     - Logging                       │
│     - Tracing                       │
│     - Metrics                       │
└────┬────────────────────────────────┘
     │
     ↓
┌─────────────────────────────────────┐
│   Router.HandleChatCompletion       │
│   (pkg/router/router.go)            │
└────┬────────────────────────────────┘
     │
     ↓ 1. Extract user_id
┌─────────────────────────────────────┐
│   Rate Limiter Check                │
│   (pkg/ratelimit/token_bucket.go)   │
│   • Check tokens available?         │
│   • If NO → 429 Too Many Requests   │
│   • If YES → Consume 1 token        │
└────┬────────────────────────────────┘
     │
     ↓ 2. Generate cache key (SHA256 hash)
┌─────────────────────────────────────┐
│   Cache Lookup (Redis)              │
│   (pkg/cache/redis.go)              │
│   • Key: chat:<hash>                │
│   • If HIT → Return cached response │
│   • If MISS → Continue              │
└────┬────────────────────────────────┘
     │ Cache MISS
     ↓ 3. Determine provider from model
┌─────────────────────────────────────┐
│   Provider Selection                │
│   • "gpt-*" → OpenAI                │
│   • "claude-*" → Anthropic          │
│   • Validate provider exists        │
└────┬────────────────────────────────┘
     │
     ↓ 4. Call provider
┌─────────────────────────────────────┐
│   LLM Provider API Call             │
│   (pkg/providers/openai.go)         │
│   • Marshal request                 │
│   • HTTP POST to provider           │
│   • Handle errors                   │
│   • Unmarshal response              │
└────┬────────────────────────────────┘
     │
     ↓ 5. Store in cache
┌─────────────────────────────────────┐
│   Cache Storage (Redis)             │
│   • Key: chat:<hash>                │
│   • Value: JSON response            │
│   • TTL: 5 minutes                  │
└────┬────────────────────────────────┘
     │
     ↓ 6. Return response
┌─────────────────────────────────────┐
│   Response to Client                │
│   • Status: 200 OK                  │
│   • Body: {id, choices, usage, ...} │
└─────────────────────────────────────┘
```

## Observability Architecture

### Tracing (OpenTelemetry + Jaeger)

```
[HTTP Request] → [Tracing Middleware]
                        ↓
                 [Create Span: "POST /v1/chat/completions"]
                        ↓
         [Add Attributes: method, url, user_agent]
                        ↓
                [Process Request]
                        ↓
      [Add Attribute: http.status_code = 200]
                        ↓
                  [End Span]
                        ↓
            [Export to Jaeger Collector]
```

**Jaeger UI** (http://localhost:16686) shows:
- Request traces across services
- Latency breakdown
- Error traces

### Metrics (Prometheus)

```
[HTTP Request] → [Metrics Middleware]
                        ↓
        [Increment: http_requests_total]
                        ↓
     [Observe: http_request_duration_seconds]
                        ↓
         [LLM Request Completes]
                        ↓
        [Increment: llm_requests_total]
        [Observe: llm_request_duration_seconds]
        [Add: llm_tokens_used_total]
                        ↓
            [Cache Hit/Miss?]
                        ↓
        [Increment: cache_hits_total OR cache_misses_total]
```

**Prometheus** scrapes `/metrics` endpoint.

### Logging (Zap)

```json
{
  "timestamp": "2026-01-30T10:15:30Z",
  "level": "info",
  "msg": "HTTP request",
  "method": "POST",
  "path": "/v1/chat/completions",
  "status": 200,
  "duration": "1.234s",
  "ip": "192.168.1.100",
  "user_agent": "curl/7.64.1"
}
```

## Caching Strategy

### Cache Key Generation

```go
// Request:
{
  "model": "gpt-4",
  "messages": [{"role": "user", "content": "Hello"}],
  "temperature": 0.7
}

// Serialized to JSON → SHA256 hash → Cache key:
"chat:a3f5e8c2d9b1..."
```

### Cache Hit/Miss Logic

```
Request → Generate Key → Redis GET
                           ↓
                    ┌──────┴───────┐
                    │              │
                   HIT            MISS
                    │              │
              Return Cached    Call Provider
              (Skip API)          ↓
                              Cache Result
                                  ↓
                             Return Response
```

### Cache Invalidation

- **TTL-based**: 5 minutes (configurable)
- **No manual invalidation** (for stateless LLM responses)

## Rate Limiting

### Token Bucket Algorithm

```
User makes request
     ↓
Check bucket[user_id]
     ↓
Refill tokens (based on time elapsed)
     ↓
tokens >= request_cost?
     ↓
    ┌───────┴───────┐
    │               │
   YES              NO
    │               │
Consume tokens   Return 429
Allow request
```

### Configuration

- **Capacity**: 100 tokens (max burst)
- **Refill Rate**: 1.67 tokens/second (100/minute)
- **Per-user**: Isolated buckets

## Error Handling

### HTTP Status Codes

| Status | Scenario |
|--------|----------|
| `200 OK` | Successful response |
| `400 Bad Request` | Invalid input |
| `401 Unauthorized` | Missing/invalid API key |
| `429 Too Many Requests` | Rate limit exceeded |
| `500 Internal Server Error` | Provider error |
| `503 Service Unavailable` | Redis/dependency failure |

### Error Flow

```
Error Occurs
     ↓
Log Error (Zap)
     ↓
Record Metric (error counter)
     ↓
Set Span Status (error)
     ↓
Return Error Response
{
  "error": "description",
  "code": "error_code"
}
```

## Deployment Architecture

### Kubernetes

```
┌─────────────────────────────────────────┐
│         Kubernetes Cluster              │
│                                         │
│  ┌────────────────────────────────┐    │
│  │  AI Gateway Deployment         │    │
│  │  • Replicas: 3                 │    │
│  │  • Resource Limits:            │    │
│  │    CPU: 500m, Mem: 512Mi       │    │
│  │  • Health Checks: /health      │    │
│  │  • Readiness: /ready           │    │
│  └────────┬───────────────────────┘    │
│           │                             │
│  ┌────────┴───────────────────────┐    │
│  │  Service (LoadBalancer)        │    │
│  │  • Port: 80 → 8080             │    │
│  └────────┬───────────────────────┘    │
│           │                             │
│  ┌────────┴───────────────────────┐    │
│  │  Redis StatefulSet             │    │
│  │  • Replicas: 1                 │    │
│  │  • Persistent Volume: 10Gi     │    │
│  └────────────────────────────────┘    │
│                                         │
│  ┌────────────────────────────────┐    │
│  │  Prometheus (monitoring)       │    │
│  └────────────────────────────────┘    │
│                                         │
│  ┌────────────────────────────────┐    │
│  │  Jaeger (tracing)              │    │
│  └────────────────────────────────┘    │
└─────────────────────────────────────────┘
```

## Security Considerations

1. **API Key Storage**: Environment variables (never in code)
2. **Rate Limiting**: Prevent abuse
3. **Input Validation**: Sanitize all inputs
4. **TLS**: HTTPS in production
5. **Network Policies**: Restrict inter-service communication
6. **Secrets Management**: Kubernetes secrets for API keys
7. **Audit Logging**: Log all requests with user IDs

## Performance Optimization

1. **Connection Pooling**: HTTP clients reuse connections
2. **Redis Pipelining**: Batch cache operations
3. **Go Routines**: Concurrent request handling
4. **Caching**: Reduce redundant API calls (40-60% hit rate)
5. **Timeouts**: Prevent hanging requests (60s)
6. **Resource Limits**: Prevent memory leaks

## Scalability

### Horizontal Scaling

- **Stateless Design**: No in-memory state (uses Redis)
- **Multiple Replicas**: Kubernetes HPA (Horizontal Pod Autoscaler)
- **Load Balancing**: Kubernetes Service distributes traffic

### Vertical Scaling

- **CPU**: Increase for high throughput
- **Memory**: Increase for large request/response payloads

## Future Enhancements

1. **Circuit Breakers**: Prevent cascading failures
2. **A/B Testing**: Route % of traffic to different models
3. **Semantic Caching**: Vector similarity for cache lookup
4. **Streaming**: WebSocket support for real-time responses
5. **Multi-tenancy**: Quotas & isolation per tenant
6. **Cost Dashboard**: Real-time cost tracking per user/model
7. **Auto-scaling**: Based on queue depth & latency

