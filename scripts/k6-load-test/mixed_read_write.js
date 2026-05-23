// PACS — Mixed read/write load test
//
// 目的：同時打 access-api (swipe write path) 與 reporting-api (報表 read path)，
//       驗證 spec 的核心矛盾——「write-heavy 不拖慢 read-heavy」CQRS 解耦。
//
// 對應規範：
//   NFR-1 寫入 P99 < 50ms (門禁決策)
//   NFR-2 報表 P95 < 200ms (聚合查詢)
//
// 預期：兩條 threshold 同時通過 → 證明 CQRS 拆分有效。

import http from 'k6/http';
import { check } from 'k6';
import { pickBadge } from './lib/badges.js';
import { nextDirection } from './lib/apb_safe.js';

const ACCESS_API = __ENV.ACCESS_API || 'http://localhost:8080';
const REPORT_API = __ENV.REPORT_API || 'http://localhost:8081';

http.setResponseCallback(http.expectedStatuses({ min: 200, max: 299 }, 401, 403));

export const options = {
  scenarios: {
    swipe_burst: {
      executor: 'ramping-arrival-rate',
      startRate: 5,
      timeUnit: '1s',
      preAllocatedVUs: 50,
      maxVUs: 200,
      exec: 'swipeFlow',
      stages: [
        { duration: '30s', target: 50 },
        { duration: '2m',  target: 50 },
        { duration: '30s', target: 5  },
      ],
    },
    report_queries: {
      executor: 'constant-arrival-rate',
      rate: 5,
      timeUnit: '1s',
      duration: '3m30s',
      preAllocatedVUs: 10,
      maxVUs: 30,
      exec: 'reportFlow',
    },
  },
  thresholds: {
    'http_req_duration{endpoint:swipe}':  ['p(99)<50'],   // NFR-1
    'http_req_duration{endpoint:report}': ['p(95)<200'],  // NFR-2
    'http_req_failed{endpoint:swipe}':    ['rate<0.05'],
    'http_req_failed{endpoint:report}':   ['rate<0.01'],
  },
  summaryTrendStats: ['min', 'avg', 'med', 'p(95)', 'p(99)', 'max'],
};

export function swipeFlow() {
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
  check(res, { 'status 200 or 403_APB': (r) => r.status === 200 || r.status === 403 });
}

const today = new Date().toISOString().slice(0, 10);
const reportPaths = [
  `/v1/reports/attendance?date=${today}`,
  `/v1/reports/manager-team?as=B-000001&date=${today}`,
  `/v1/reports/trend?period=day&as=B-000001`,
];

export function reportFlow() {
  const path = reportPaths[Math.floor(Math.random() * reportPaths.length)];
  const res = http.get(`${REPORT_API}${path}`, {
    tags: { endpoint: 'report' },
  });
  check(res, { 'status 200/401/403': (r) => [200, 401, 403].includes(r.status) });
}
