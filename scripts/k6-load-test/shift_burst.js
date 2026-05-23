// PACS — Shift Change Burst load test
//
// 對應規範：
//   HW2 §4.2 Phase 2: 換班尖峰 8:00 AM ~20K events in 15 minutes (80/20 法則)
//   Distributed Physical Access Control System: "Shift Change spike" 需 Grafana 可視化
//
// 場景：ramping-arrival-rate 從 5 QPS → 100 QPS（15 min plateau）→ 5 QPS
// 自動驗證：NFR-1 P99 < 50ms (門禁決策延遲)

import http from 'k6/http';
import { check } from 'k6';
import { pickBadge } from './lib/badges.js';
import { nextDirection } from './lib/apb_safe.js';

const ACCESS_API = __ENV.ACCESS_API || 'http://localhost:8080';

http.setResponseCallback(http.expectedStatuses({ min: 200, max: 299 }, 403));

export const options = {
  scenarios: {
    shift_burst: {
      executor: 'ramping-arrival-rate',
      startRate: 1,
      timeUnit: '1s',
      preAllocatedVUs: 50,
      maxVUs: 200,
      stages: [
        { duration: '30s', target: 5   }, // 常態 baseline
        { duration: '30s', target: 100 }, // ramp-up to shift peak
        { duration: '2m',  target: 100 }, // 維持換班尖峰（demo 用 2 min；production 跑 15 min）
        { duration: '30s', target: 5   }, // ramp-down
      ],
    },
  },
  thresholds: {
    // NFR-1：門禁決策 P99 < 50ms
    'http_req_duration{endpoint:swipe}': ['p(99)<50'],
    'http_req_failed{endpoint:swipe}':   ['rate<0.05'], // 容忍少量 APB reject
  },
  summaryTrendStats: ['min', 'avg', 'med', 'p(95)', 'p(99)', 'max'],
};

const gates = ['G-01', 'G-02', 'G-03', 'G-04', 'CR-01', 'OFF-01'];
const sites = ['FAB12A', 'FAB12B'];

export default function () {
  const badge = pickBadge();
  const payload = JSON.stringify({
    badge_id:  badge,
    site_id:   sites[Math.floor(Math.random() * sites.length)],
    gate_id:   gates[Math.floor(Math.random() * gates.length)],
    direction: nextDirection(badge),
  });

  const res = http.post(`${ACCESS_API}/v1/swipe`, payload, {
    headers: { 'Content-Type': 'application/json' },
    tags:    { endpoint: 'swipe' },
  });

  // 200 = success, 403 = anti-passback reject（也是合法回應，不算錯）
  check(res, {
    'status 200 or 403_APB': (r) => r.status === 200 || r.status === 403,
  });
}
