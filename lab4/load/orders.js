import http from 'k6/http';
import { check } from 'k6';

const baseURL = __ENV.BASE_URL;
if (!baseURL) {
  throw new Error('Set BASE_URL to the shop API address');
}

// Keep the offered request rate stable even when the application slows down.
export const options = {
  scenarios: {
    orders: {
      executor: 'constant-arrival-rate',
      rate: Number(__ENV.RATE || 20),
      timeUnit: '1s',
      duration: __ENV.DURATION || '5m',
      preAllocatedVUs: 20,
      maxVUs: 100,
      gracefulStop: '10s',
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.01'],
    checks: ['rate>0.99'],
    dropped_iterations: ['count<1'],
  },
};

http.setResponseCallback(http.expectedStatuses(201));

export default function () {
  const response = http.post(
    `${baseURL}/order`,
    JSON.stringify({ item: `load-${__VU}-${__ITER}` }),
    {
      headers: { 'Content-Type': 'application/json' },
      tags: { name: 'POST /order' },
      timeout: '10s',
    },
  );
  check(response, { 'order created (201)': (r) => r.status === 201 });
}
