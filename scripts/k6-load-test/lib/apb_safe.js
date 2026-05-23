// Anti-Passback friendly direction picker.
//
// access-api 的 Anti-Passback 規則：同 badge 30 秒內同方向會被拒絕（REJECTED_APB）。
// 為了讓壓測統計乾淨（避免 50%+ 流量被預期 reject 干擾延遲分布），
// 我們維持每個 badge 的 last direction，下一次自動切換。
//
// 注意：k6 每個 VU 有獨立的 module scope，跨 VU 間不同步 — 但 BADGE_COUNT
// 通常遠大於 VU 數，碰撞機率低，可接受少量真實 APB reject（壓測也應該驗到）。

const lastDir = new Map();

export function nextDirection(badge) {
  const prev = lastDir.get(badge);
  const next = prev === 'IN' ? 'OUT' : 'IN';
  lastDir.set(badge, next);
  return next;
}
