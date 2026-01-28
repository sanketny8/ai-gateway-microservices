// Load test script for K6
import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate } from 'k6/metrics';

const errorRate = new Rate('errors');

export let options = {
  stages: [
    { duration: '2m', target: 100 }, // Ramp up to 100 users
    { duration: '5m', target: 100 }, // Stay at 100 users
    { duration: '2m', target: 200 }, // Ramp up to 200
    { duration: '5m', target: 200 }, // Stay at 200
    { duration: '2m', target: 0 },   // Ramp down
  ],
  thresholds: {
    http_req_duration: ['p(95)<2000'], // 95% of requests under 2s
    errors: ['rate<0.05'],              // Error rate < 5%
  },
};

export default function () {
  const url = 'http://localhost:8080/v1/chat/completions';
  const payload = JSON.stringify({
    model: 'gpt-4',
    messages: [
      { role: 'user', content: 'What is the capital of France?' }
    ],
    max_tokens: 50
  });

  const params = {
    headers: {
      'Content-Type': 'application/json',
      'Authorization': 'Bearer test-api-key',
    },
  };

  const res = http.post(url, payload, params);
  
  check(res, {
    'status is 200': (r) => r.status === 200,
    'has response': (r) => r.body.length > 0,
    'latency < 2s': (r) => r.timings.duration < 2000,
  });

  errorRate.add(res.status !== 200);

  sleep(1);
}

/*
Run with:
k6 run --vus 100 --duration 30s load_test.js

Expected results:
- p50 latency: < 500ms (cache hit) / < 1000ms (cache miss)
- p95 latency: < 1.5s
- p99 latency: < 2.5s
- Throughput: 100+ req/s
- Error rate: < 1%
*/

