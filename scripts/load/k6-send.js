import http from 'k6/http';
import { check } from 'k6';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
const TOKEN = __ENV.TOKEN || 'demo-token-account-a';
// Default 80 RPS fits the default rate limit (100 req/s per account). For ~1K RPS use
// make load-test-stress after raising RATE_LIMIT_LIMIT or disabling rate limiting.
const TARGET_RPS = Number(__ENV.TARGET_RPS || 80);
const DURATION = __ENV.DURATION || '30s';

export const options = {
  scenarios: {
    send_load: {
      executor: 'constant-arrival-rate',
      rate: TARGET_RPS,
      timeUnit: '1s',
      duration: DURATION,
      preAllocatedVUs: Math.min(TARGET_RPS, 200),
      maxVUs: Math.max(TARGET_RPS * 2, 500),
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.01'],
    http_req_duration: ['p(99)<500'],
    checks: ['rate>0.99'],
  },
};

export function setup() {
  const topupRes = http.post(
    `${BASE_URL}/v1/account/topup`,
    JSON.stringify({ amount: 1_000_000 }),
    {
      headers: {
        'Content-Type': 'application/json',
        'X-Account-Token': TOKEN,
      },
    },
  );

  check(topupRes, {
    'topup succeeded': (r) => r.status === 200,
  });

  return { token: TOKEN };
}

export default function () {
  const payload = JSON.stringify({
    to: '+989121234567',
    body: 'k6 load test',
    message_type: 'standard',
  });

  const res = http.post(`${BASE_URL}/v1/sms/send`, payload, {
    headers: {
      'Content-Type': 'application/json',
      'X-Account-Token': TOKEN,
    },
    tags: { name: 'send_sms' },
  });

  check(res, {
    'status is 202': (r) => r.status === 202,
    'has message_id': (r) => {
      try {
        const body = JSON.parse(r.body);
        return Boolean(body.message_id);
      } catch {
        return false;
      }
    },
  });
}
