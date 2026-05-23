// PACS — Steady baseline load test
//
// 目的：常態 QPS（HW2 §4.2 Phase 2 平均 ~3.5 QPS）持續 3 分鐘，
//       量測無尖峰下的基準延遲，作為 shift_burst 的對照組。

import http from 'k6/http';
import { check } from 'k6';
import { pickBadge } from './lib/badges.js';
import { nextDirection } from './lib/apb_safe.js';

const ACCESS_API = __ENV.ACCESS_API || 'http://localhost:8080';

http.setResponseCallback(http.expectedStatuses({ min: 200, max: 299 }, 403));

export const options = {
  scenarios: {
    steady: {
      executor: 'constant-arrival-rate',
      rate: 10,
      timeUnit: '1s',
      duration: '3m',
      preAllocatedVUs: 20,
      maxVUs: 50,
    },
  },
  thresholds: {
    'http_req_duration{endpoint:swipe}': ['p(99)<50'],   // NFR-1
    'http_req_failed{endpoint:swipe}':   ['rate<0.01'],
  },
  summaryTrendStats: ['min', 'avg', 'med', 'p(95)', 'p(99)', 'max'],
};

export default function () {
  const badge = pickBadge();
  const payload = JSON.stringify({
    badge_id:  badge,
    site_id:   'FAB12A',
    gate_id:   'G-01',
    direction: nextDirection(badge),
  });

  const res = http.post(`${ACCESS_API}/v1/swipe`, payload, {
    headers: { 'Content-Type': 'application/json' },
    tags:    { endpoint: 'swipe' },
  });

  check(res, {
    'status 200 or 403_APB': (r) => r.status === 200 || r.status === 403,
  });
}
