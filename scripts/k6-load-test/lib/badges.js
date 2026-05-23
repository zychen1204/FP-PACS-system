// Random badge_id pool — matches seed-generator employee naming `B-NNNNNN`.
// Size controlled by env BADGE_COUNT (default 1000 = Phase 1).
const BADGE_COUNT = parseInt(__ENV.BADGE_COUNT || '1000', 10);

export function pickBadge() {
  // Skip B-000001 (廠長 L1) to avoid manager bias; B-000002 onward.
  const id = Math.floor(Math.random() * (BADGE_COUNT - 1)) + 2;
  return `B-${String(id).padStart(6, '0')}`;
}

export function totalBadges() {
  return BADGE_COUNT;
}
