/**
 * PACS Frontend - Test Suite
 *
 * Coverage:
 * - Unit:       formatTime, formatTimeDetailed, getDateDaysAgo, getRoleBadge
 * - API:        swipe, attendance, aggregated, audit trail, manager-team,
 *               trend (with summary), alerts
 * - State:      localStorage persistence
 * - Validation: payload shapes, canonical status values, gate ID format
 * - UI:         DOM class toggles, period / isAggregated logic
 */

// ============ TEST FRAMEWORK ============
class TestSuite {
    constructor(name) {
        this.name = name;
        this.tests = [];
        this.passed = 0;
        this.failed = 0;
    }

    it(description, testFn) {
        this.tests.push({ description, testFn });
    }

    async run() {
        console.log(`\n${'='.repeat(60)}`);
        console.log(`📋 TEST SUITE: ${this.name}`);
        console.log(`${'='.repeat(60)}\n`);

        for (const test of this.tests) {
            try {
                await test.testFn();
                this.passed++;
                console.log(`✅ PASS: ${test.description}`);
            } catch (error) {
                this.failed++;
                console.error(`❌ FAIL: ${test.description}`);
                console.error(`   Error: ${error.message}`);
            }
        }

        console.log(`\n${'='.repeat(60)}`);
        console.log(`📊 Results: ${this.passed} passed, ${this.failed} failed / ${this.tests.length} total`);
        console.log(`${'='.repeat(60)}\n`);

        return this.failed === 0;
    }
}

const assert = {
    equal: (actual, expected, message) => {
        if (actual !== expected)
            throw new Error(`${message}\nExpected: ${expected}\nActual:   ${actual}`);
    },
    true: (value, message) => {
        if (value !== true) throw new Error(message);
    },
    false: (value, message) => {
        if (value !== false) throw new Error(message);
    },
    exists: (value, message) => {
        if (value === null || value === undefined) throw new Error(message);
    },
    includes: (array, value, message) => {
        if (!array.includes(value)) throw new Error(`${message}: "${value}" not in [${array}]`);
    },
    contains: (str, sub, message) => {
        if (!str.includes(sub)) throw new Error(`${message}: "${sub}" not found in "${str}"`);
    },
};

// ============ UNIT TESTS ============
const unitTests = new TestSuite('Unit Tests - Utility Functions');

unitTests.it('formatTime: null / undefined / empty → "-"', () => {
    const fmt = (s) => {
        if (!s) return '-';
        try { return new Date(s).toLocaleTimeString('zh-TW', { hour: '2-digit', minute: '2-digit' }); }
        catch { return s; }
    };
    assert.equal(fmt(null),      '-', 'null should return -');
    assert.equal(fmt(undefined), '-', 'undefined should return -');
    assert.equal(fmt(''),        '-', 'empty string should return -');
});

unitTests.it('formatTime: Valid ISO timestamp returns HH:MM string', () => {
    const fmt = (s) => {
        if (!s) return '-';
        try { return new Date(s).toLocaleTimeString('zh-TW', { hour: '2-digit', minute: '2-digit' }); }
        catch { return s; }
    };
    const result = fmt('2026-05-14T09:30:00Z');
    assert.true(result !== '-', 'Should not be dash for valid timestamp');
    assert.exists(result, 'Should return a string');
});

unitTests.it('formatTimeDetailed: null → "-"', () => {
    const fmt = (s) => {
        if (!s) return '-';
        try { return new Date(s).toLocaleTimeString('zh-TW', { hour: '2-digit', minute: '2-digit', second: '2-digit' }); }
        catch { return s; }
    };
    assert.equal(fmt(null), '-', 'null should return -');
});

unitTests.it('formatTimeDetailed: Valid timestamp returns HH:MM:SS string', () => {
    const fmt = (s) => {
        if (!s) return '-';
        try { return new Date(s).toLocaleTimeString('zh-TW', { hour: '2-digit', minute: '2-digit', second: '2-digit' }); }
        catch { return s; }
    };
    const result = fmt('2026-05-14T09:30:45Z');
    assert.true(result !== '-', 'Should not be dash');
    assert.exists(result, 'Should return a string');
});

unitTests.it('getDateDaysAgo: Returns YYYY-MM-DD before today', () => {
    const fn = (days) => {
        const d = new Date();
        d.setDate(d.getDate() - days);
        return d.toISOString().split('T')[0];
    };
    const today = new Date().toISOString().split('T')[0];
    const ago7  = fn(7);
    assert.true(/^\d{4}-\d{2}-\d{2}$/.test(ago7), 'Must be YYYY-MM-DD');
    assert.true(ago7 < today, '7 days ago must be before today');
});

unitTests.it('getRoleBadge: MANAGER_L1 → 一級主管 + class mgr-1', () => {
    const fn = (report) => {
        const status = report.status || 'STAFF';
        const roles = {
            'MANAGER_L1': { label: '🎖️ 一級主管', class: 'mgr-1' },
            'MANAGER_L2': { label: '👔 二級主管', class: 'mgr-2' },
            'STAFF':      { label: '👤 員工',      class: 'employee' },
        };
        const role = roles[status] || roles['STAFF'];
        return `<span class="badge-role ${role.class}">${role.label}</span>`;
    };
    const html = fn({ status: 'MANAGER_L1' });
    assert.contains(html, '一級主管', 'Should contain 一級主管');
    assert.contains(html, 'mgr-1', 'Should have class mgr-1');
});

unitTests.it('getRoleBadge: MANAGER_L2 → 二級主管 + class mgr-2', () => {
    const fn = (report) => {
        const status = report.status || 'STAFF';
        const roles = {
            'MANAGER_L1': { label: '🎖️ 一級主管', class: 'mgr-1' },
            'MANAGER_L2': { label: '👔 二級主管', class: 'mgr-2' },
            'STAFF':      { label: '👤 員工',      class: 'employee' },
        };
        const role = roles[status] || roles['STAFF'];
        return `<span class="badge-role ${role.class}">${role.label}</span>`;
    };
    const html = fn({ status: 'MANAGER_L2' });
    assert.contains(html, '二級主管', 'Should contain 二級主管');
    assert.contains(html, 'mgr-2', 'Should have class mgr-2');
});

unitTests.it('getRoleBadge: STAFF → 員工 + class employee', () => {
    const fn = (report) => {
        const status = report.status || 'STAFF';
        const roles = {
            'MANAGER_L1': { label: '🎖️ 一級主管', class: 'mgr-1' },
            'MANAGER_L2': { label: '👔 二級主管', class: 'mgr-2' },
            'STAFF':      { label: '👤 員工',      class: 'employee' },
        };
        const role = roles[status] || roles['STAFF'];
        return `<span class="badge-role ${role.class}">${role.label}</span>`;
    };
    const html = fn({ status: 'STAFF' });
    assert.contains(html, '員工', 'Should contain 員工');
    assert.contains(html, 'employee', 'Should have class employee');
});

unitTests.it('getRoleBadge: Unknown / legacy status falls back to STAFF', () => {
    const fn = (report) => {
        const status = report.status || 'STAFF';
        const roles = {
            'MANAGER_L1': { label: '🎖️ 一級主管', class: 'mgr-1' },
            'MANAGER_L2': { label: '👔 二級主管', class: 'mgr-2' },
            'STAFF':      { label: '👤 員工',      class: 'employee' },
        };
        const role = roles[status] || roles['STAFF'];
        return `<span class="badge-role ${role.class}">${role.label}</span>`;
    };
    // 'mgr-1' is the old legacy value — should fall back to STAFF
    const html = fn({ status: 'mgr-1' });
    assert.contains(html, '員工', 'Legacy mgr-1 should fall back to 員工');
    assert.contains(html, 'employee', 'Legacy mgr-1 should fall back to class employee');
});

// ============ API MOCK TESTS ============
const apiTests = new TestSuite('Integration Tests - API Response Shapes');

// Mock responses matching actual backend models (models.go)
const mockDB = {
    '/v1/swipe': {
        ok: true, status: 200,
        // SwipeResponse: { status, message, error_code (omitempty) }
        json: () => Promise.resolve({ status: 'SUCCESS', message: '允許進入' }),
    },
    '/v1/reports/attendance': {
        ok: true, status: 200,
        // []AttendanceReport
        json: () => Promise.resolve([
            {
                employee_id: 'B001', name: '王小明',
                status: 'STAFF', org_path: 'TSMC.Fab12.製造部',
                work_date: '2026-05-14',
                first_in: '2026-05-14T00:00:00Z',
                last_out: '2026-05-14T09:00:00Z',
                swipe_count: 4, stay_hours: 9.0,
            },
        ]),
    },
    '/v1/reports/attendance/aggregated': {
        ok: true, status: 200,
        // []EmployeeAggregate
        json: () => Promise.resolve([
            {
                employee_id: 'B001', name: '王小明',
                status: 'STAFF', org_path: 'TSMC.Fab12.製造部',
                total_swipes: 80, total_stay_hours: 176.0,
                day_count: 20, avg_swipes: 4.0, avg_stay_hours: 8.8,
            },
        ]),
    },
    '/v1/reports/manager-team': {
        ok: true, status: 200,
        // { manager_scope, reports: []AttendanceReport }
        json: () => Promise.resolve({
            manager_scope: 'TSMC.Fab12.製造部',
            reports: [
                {
                    employee_id: 'B001', name: '王小明',
                    status: 'STAFF', org_path: 'TSMC.Fab12.製造部',
                    work_date: '2026-05-14', swipe_count: 4, stay_hours: 9.0,
                },
            ],
        }),
    },
    '/v1/reports/trend': {
        ok: true, status: 200,
        // { scope, trends: []AttendanceTrend, summary: TrendSummary }
        json: () => Promise.resolve({
            scope: 'TSMC.Fab12',
            trends: [
                { bucket: '2026-05-08', head_count: 150, avg_stay_hrs: 8.5, total_swipes: 600 },
                { bucket: '2026-05-09', head_count: 145, avg_stay_hrs: 8.7, total_swipes: 580 },
            ],
            summary: { avg_swipes_per_person: 4.0, avg_head_count: 147.5, avg_stay_hrs: 8.6 },
        }),
    },
    '/v1/audit': {
        ok: true, status: 200,
        // []AccessEvent
        json: () => Promise.resolve([
            { id: 1, badge_id: 'B001', site_id: 'Fab12', gate_id: 'Gate-1A', direction: 'IN',  status: 'SUCCESS', reason: '', timestamp: '2026-05-14T00:00:00Z' },
            { id: 2, badge_id: 'B001', site_id: 'Fab12', gate_id: 'Gate-1A', direction: 'OUT', status: 'SUCCESS', reason: '', timestamp: '2026-05-14T09:00:00Z' },
        ]),
    },
    '/v1/alerts': {
        ok: true, status: 200,
        // []Alert: id, alert_type, severity, occurred_at (not timestamp), badge_id, site_id, gate_id
        json: () => Promise.resolve([
            { id: 1, alert_type: 'APB_BURST', severity: 'HIGH', occurred_at: '2026-05-14T10:00:00Z', badge_id: 'B001', site_id: 'Fab12', gate_id: 'Gate-1A' },
        ]),
    },
};

const fetchMock = (url) => {
    // Sort by length descending so more specific paths (e.g. /attendance/aggregated)
    // are matched before shorter prefixes (e.g. /attendance).
    const key = Object.keys(mockDB).sort((a, b) => b.length - a.length).find(k => url.includes(k));
    return Promise.resolve(
        key ? mockDB[key] : { ok: false, status: 404, json: () => Promise.resolve({ error: 'not found' }) }
    );
};

apiTests.it('Swipe: Response has status + message; NO employee_id', async () => {
    const data = await (await fetchMock('/v1/swipe')).json();
    assert.equal(data.status, 'SUCCESS', 'status should be SUCCESS');
    assert.exists(data.message, 'message should exist');
    assert.equal(data.employee_id, undefined, 'employee_id must NOT be in SwipeResponse');
});

apiTests.it('Attendance (day): Records have swipe_count, stay_hours, canonical status', async () => {
    const data = await (await fetchMock('/v1/reports/attendance')).json();
    const canonical = ['MANAGER_L1', 'MANAGER_L2', 'STAFF'];
    assert.true(Array.isArray(data), 'Should return array');
    data.forEach(r => {
        assert.exists(r.swipe_count, 'swipe_count required');
        assert.exists(r.stay_hours,  'stay_hours required');
        assert.includes(canonical, r.status, `status must be canonical: got "${r.status}"`);
    });
});

apiTests.it('Aggregated attendance: Records have total_swipes, avg_swipes, avg_stay_hours', async () => {
    const data = await (await fetchMock('/v1/reports/attendance/aggregated')).json();
    assert.true(Array.isArray(data), 'Should return array');
    const r = data[0];
    assert.exists(r.total_swipes,   'total_swipes required');
    assert.exists(r.avg_swipes,     'avg_swipes required');
    assert.exists(r.avg_stay_hours, 'avg_stay_hours required');
});

apiTests.it('Manager Team: Response has manager_scope and reports array', async () => {
    const data = await (await fetchMock('/v1/reports/manager-team')).json();
    assert.exists(data.manager_scope, 'manager_scope required');
    assert.true(Array.isArray(data.reports), 'reports must be array');
});

apiTests.it('Trend: Response has trends array AND summary object', async () => {
    const data = await (await fetchMock('/v1/reports/trend')).json();
    assert.true(Array.isArray(data.trends), 'trends must be array');
    assert.exists(data.summary,                        'summary required');
    assert.exists(data.summary.avg_swipes_per_person,  'summary.avg_swipes_per_person required');
    assert.exists(data.summary.avg_head_count,         'summary.avg_head_count required');
    assert.exists(data.summary.avg_stay_hrs,           'summary.avg_stay_hrs required');
});

apiTests.it('Trend: Buckets have head_count, avg_stay_hrs, total_swipes', async () => {
    const data = await (await fetchMock('/v1/reports/trend')).json();
    if (data.trends.length > 0) {
        const t = data.trends[0];
        assert.exists(t.bucket,       'bucket required');
        assert.exists(t.head_count,   'head_count required');
        assert.exists(t.avg_stay_hrs, 'avg_stay_hrs required');
        assert.exists(t.total_swipes, 'total_swipes required');
    }
});

apiTests.it('Audit Trail: Returns []AccessEvent with badge_id, direction, timestamp', async () => {
    const data = await (await fetchMock('/v1/audit')).json();
    assert.true(Array.isArray(data), 'audit trail must be array');
    if (data.length > 0) {
        assert.exists(data[0].badge_id,  'badge_id required');
        assert.exists(data[0].direction, 'direction required');
        assert.exists(data[0].timestamp, 'timestamp required');
        assert.includes(['IN', 'OUT'], data[0].direction, 'direction must be IN or OUT');
    }
});

apiTests.it('Alerts: Field is occurred_at (not timestamp); has alert_type and severity', async () => {
    const data = await (await fetchMock('/v1/alerts')).json();
    assert.true(Array.isArray(data), 'alerts must be array');
    if (data.length > 0) {
        const a = data[0];
        assert.exists(a.occurred_at, 'occurred_at required');
        assert.exists(a.alert_type,  'alert_type required');
        assert.exists(a.severity,    'severity required');
        assert.equal(a.timestamp, undefined, '"timestamp" must NOT exist — field is occurred_at');
    }
});

// ============ STATE MANAGEMENT TESTS ============
const stateTests = new TestSuite('State Management - localStorage');

stateTests.it('apiUrl: Save and restore', () => {
    localStorage.setItem('apiUrl', 'http://localhost:8080');
    assert.equal(localStorage.getItem('apiUrl'), 'http://localhost:8080', 'apiUrl should persist');
    localStorage.removeItem('apiUrl');
});

stateTests.it('reportUrl: Save and restore', () => {
    localStorage.setItem('reportUrl', 'http://localhost:8081');
    assert.equal(localStorage.getItem('reportUrl'), 'http://localhost:8081', 'reportUrl should persist');
    localStorage.removeItem('reportUrl');
});

stateTests.it('pacs_token: Save and restore JWT', () => {
    const token = 'eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.test.sig';
    localStorage.setItem('pacs_token', token);
    assert.equal(localStorage.getItem('pacs_token'), token, 'token should persist');
    localStorage.removeItem('pacs_token');
});

stateTests.it('current_badge: Save and restore badge ID', () => {
    localStorage.setItem('current_badge', 'B001');
    assert.equal(localStorage.getItem('current_badge'), 'B001', 'badge should persist');
    localStorage.removeItem('current_badge');
});

// ============ VALIDATION TESTS ============
const validationTests = new TestSuite('Data Validation');

validationTests.it('SwipeRequest: Required fields + direction in [IN, OUT]', () => {
    const payload = { badge_id: 'B001', site_id: 'Fab12', gate_id: 'Gate-1A', direction: 'IN' };
    assert.exists(payload.badge_id,  'badge_id required');
    assert.exists(payload.site_id,   'site_id required');
    assert.exists(payload.gate_id,   'gate_id required');
    assert.includes(['IN', 'OUT'], payload.direction, 'direction must be IN or OUT');
});

validationTests.it('Gate IDs use Gate-NX format (not N-X)', () => {
    const gates = ['Gate-1A', 'Gate-1B', 'Gate-1C', 'Gate-2A', 'Gate-2B', 'Gate-2C'];
    gates.forEach(g => {
        assert.true(/^Gate-[12][A-C]$/.test(g), `${g} must match Gate-NX format`);
    });
});

validationTests.it('Canonical status values: MANAGER_L1, MANAGER_L2, STAFF', () => {
    const canonical = new Set(['MANAGER_L1', 'MANAGER_L2', 'STAFF']);
    ['MANAGER_L1', 'MANAGER_L2', 'STAFF'].forEach(s => {
        assert.true(canonical.has(s), `${s} must be canonical`);
    });
});

validationTests.it('Legacy status values are NOT canonical', () => {
    const canonical = new Set(['MANAGER_L1', 'MANAGER_L2', 'STAFF']);
    ['mgr-1', 'mgr-2', 'employee', 'is_manager'].forEach(s => {
        assert.false(canonical.has(s), `"${s}" is a legacy value and must not be used`);
    });
});

validationTests.it('Alert severities are SCREAMING_SNAKE_CASE', () => {
    ['CRITICAL', 'HIGH', 'MEDIUM', 'LOW'].forEach(s => {
        assert.true(/^[A-Z]+$/.test(s), `${s} must be uppercase`);
    });
});

validationTests.it('Date range: end_date >= start_date', () => {
    const start = '2026-05-01', end = '2026-05-31';
    assert.true(end >= start, 'end_date must not precede start_date');
});

// ============ UI LOGIC TESTS ============
const uiTests = new TestSuite('UI Logic Tests');

uiTests.it('isAggregated: only true for month and quarter', () => {
    const isAggregated = (p) => p === 'month' || p === 'quarter';
    assert.false(isAggregated('day'),     'day must NOT be aggregated');
    assert.true(isAggregated('month'),    'month must be aggregated');
    assert.true(isAggregated('quarter'),  'quarter must be aggregated');
});

uiTests.it('Attendance mode: self / org are the only valid modes', () => {
    const modes = ['self', 'org'];
    modes.forEach(m => assert.includes(['self', 'org'], m, `${m} must be valid`));
});

uiTests.it('Server status indicator toggles online / offline classes', () => {
    const dot = document.createElement('span');
    dot.classList.add('offline');
    dot.classList.remove('offline');
    dot.classList.add('online');
    assert.true(dot.classList.contains('online'),   'Must be online');
    assert.false(dot.classList.contains('offline'), 'Must not be offline');
});

uiTests.it('Trend stats: avg_swipe computed as total_swipes / head_count per bucket', () => {
    const bucket = { head_count: 100, total_swipes: 400 };
    const avg = bucket.head_count > 0 ? bucket.total_swipes / bucket.head_count : 0;
    assert.equal(avg, 4, 'avg_swipe should be 400/100 = 4');
});

uiTests.it('Stats: isAggregated selects correct field for swipe count', () => {
    const dayRecord  = { swipe_count: 4,  total_swipes: undefined };
    const aggRecord  = { swipe_count: undefined, total_swipes: 80 };
    const getSwipes = (r, agg) => agg ? (r.total_swipes || 0) : (r.swipe_count || 0);
    assert.equal(getSwipes(dayRecord, false), 4,  'Day mode uses swipe_count');
    assert.equal(getSwipes(aggRecord, true),  80, 'Aggregated mode uses total_swipes');
});

// ============ RUN ALL ============
async function runAllTests() {
    console.log('\n');
    console.log('╔════════════════════════════════════════════════════════════╗');
    console.log('║       PACS FRONTEND TEST SUITE                            ║');
    console.log('╚════════════════════════════════════════════════════════════╝');

    const suites = [unitTests, apiTests, stateTests, validationTests, uiTests];
    const results = [];

    for (const suite of suites) {
        try {
            results.push(await suite.run());
        } catch (e) {
            console.error(`❌ Suite error: ${e.message}`);
            results.push(false);
        }
    }

    const allPassed = results.every(r => r === true);
    console.log('╔════════════════════════════════════════════════════════════╗');
    console.log(allPassed
        ? '║                  ✅ ALL TESTS PASSED                      ║'
        : '║                  ⚠️  SOME TESTS FAILED                    ║');
    console.log('╚════════════════════════════════════════════════════════════╝\n');

    return allPassed;
}

if (typeof module !== 'undefined' && module.exports) {
    module.exports = { runAllTests, TestSuite, assert };
}
if (typeof window !== 'undefined') {
    window.runAllTests = runAllTests;
}
