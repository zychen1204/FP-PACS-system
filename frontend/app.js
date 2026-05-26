// ============================================
// PACS Frontend — app.js
// (logic unchanged; chart colours updated for light theme)
// ============================================

const state = {
    apiUrl:    localStorage.getItem('apiUrl')    || 'http://localhost:8080',
    reportUrl: localStorage.getItem('reportUrl') || 'http://localhost:8081',
    token:     localStorage.getItem('pacs_token') || null,
    currentBadge: localStorage.getItem('current_badge') || 'B001',
    swipeHistory: JSON.parse(localStorage.getItem('swipeHistory')) || [],
    trendChart: null, modalChart: null,
    modalTrendData: null, modalPersonalData: null,
    modalBadge: null, modalScope: null, modalOrgPeriod: null,
    selectedTier: 'outer', selectedDirection: 'IN',
    serverOnline: false
};

// ── Chart colours (light bg) ─────────────────
const CHART_OPTS = {
    textColor:  '#1c1c1f',
    mutedColor: '#8c8c99',
    gridColor:  'rgba(228,228,231,.6)',
};

function getRoleBadge(report) {
    const status = report.status || 'STAFF';
    const roles = {
        'MANAGER_L1': { label: '一級主管', class: 'mgr-1' },
        'MANAGER_L2': { label: '二級主管', class: 'mgr-2' },
        'STAFF':      { label: '員工',      class: 'employee' }
    };
    const role = roles[status] || roles['STAFF'];
    return `<span class="badge-role ${role.class}">${role.label}</span>`;
}

document.addEventListener('DOMContentLoaded', () => { initializeApp(); });

function initializeApp() {
    setupEventListeners();
    restoreSettings();
    testServerConnection();
    loadSwipeHistory();
}

function loadSwipeHistory() {} // no-op (history panel removed in redesign)

function setupEventListeners() {
    document.querySelectorAll('.nav-item').forEach(item => item.addEventListener('click', switchTab));
    document.querySelectorAll('.tier-btn').forEach(btn => btn.addEventListener('click', selectTier));
    document.querySelectorAll('.direction-btn').forEach(btn => btn.addEventListener('click', selectDirection));

    document.getElementById('btn-swipe')?.addEventListener('click', sendSwipe);

    document.getElementById('btn-fetch-attendance')?.addEventListener('click', fetchAttendance);
    document.getElementById('btn-export-attendance')?.addEventListener('click', exportAttendanceExcel);
    document.getElementById('btn-org-trend')?.addEventListener('click', showOrgTrend);
    document.querySelectorAll('.period-btn').forEach(btn => btn.addEventListener('click', selectPeriod));

    document.getElementById('btn-close-trend-modal')?.addEventListener('click', closeTrendModal);
    document.getElementById('trend-modal')?.addEventListener('click', e => {
        if (e.target.id === 'trend-modal') closeTrendModal();
    });

    initYearSelects();

    document.getElementById('btn-fetch-alerts')?.addEventListener('click', fetchAlerts);
    document.getElementById('btn-save-settings')?.addEventListener('click', saveSettings);
    document.getElementById('btn-test-connection')?.addEventListener('click', testServerConnection);
    document.getElementById('btn-dev-login')?.addEventListener('click', devLogin);
}

// ── Tab switching ─────────────────────────────
function switchTab(e) {
    e.preventDefault();
    const tabId = e.target.closest('.nav-item')?.getAttribute('data-tab');
    if (!tabId) return;
    document.querySelectorAll('.nav-item').forEach(i => i.classList.remove('active'));
    e.target.closest('.nav-item').classList.add('active');
    document.querySelectorAll('.tab-content').forEach(t => t.classList.remove('active'));
    document.getElementById(tabId)?.classList.add('active');
    const titles = {
        'swipe-tab': '刷卡模擬',
        'attendance-tab': '出席報表',
        'alerts-tab': '警報異常',
        'settings-tab': '系統設定'
    };
    const pageTitle = document.getElementById('page-title');
    if (pageTitle) pageTitle.textContent = titles[tabId] || '刷卡模擬';
}

// ── Tier / Direction ──────────────────────────
function selectTier(e) {
    e.preventDefault();
    const tier = e.target.closest('.tier-btn')?.getAttribute('data-tier');
    if (!tier) return;
    document.querySelectorAll('.tier-btn').forEach(b => b.classList.remove('active'));
    e.target.closest('.tier-btn').classList.add('active');
    state.selectedTier = tier;
    const gateSelect = document.getElementById('gate-id');
    if (gateSelect) {
        gateSelect.innerHTML = tier === 'outer' ? `
            <option value="Gate-1A">Gate-1A（外層）</option>
            <option value="Gate-1B">Gate-1B（外層）</option>
            <option value="Gate-1C">Gate-1C（外層）</option>
        ` : `
            <option value="Gate-2A">Gate-2A（內層）</option>
            <option value="Gate-2B">Gate-2B（內層）</option>
            <option value="Gate-2C">Gate-2C（內層）</option>
        `;
    }
}

function selectDirection(e) {
    e.preventDefault();
    const direction = e.target.closest('.direction-btn')?.getAttribute('data-direction');
    if (!direction) return;
    document.querySelectorAll('.direction-btn').forEach(b => b.classList.remove('active'));
    e.target.closest('.direction-btn').classList.add('active');
    state.selectedDirection = direction;
}

// ── API helpers ───────────────────────────────
function getApiUrl()    { return state.apiUrl    || window.location.origin; }
function getReportUrl() { return state.reportUrl || window.location.origin; }

// ── Swipe ─────────────────────────────────────
async function sendSwipe() {
    const badgeId = document.getElementById('badge-id')?.value?.trim();
    const siteId  = document.getElementById('site-id')?.value || 'Fab12';
    const gateId  = document.getElementById('gate-id')?.value;
    if (!badgeId) { alert('請輸入員工證件 ID'); return; }
    const payload = { badge_id: badgeId, site_id: siteId, gate_id: gateId, direction: state.selectedDirection };
    try {
        const response = await fetch(`${getApiUrl()}/v1/swipe`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload)
        });
        const data = await response.json();
        displaySwipeResponse(response.status, data, payload);
    } catch (error) {
        displaySwipeResponse(0, { error: error.message }, payload);
    }
}

function displaySwipeResponse(status, data, payload) {
    const idleDiv    = document.getElementById('swipe-idle');
    const successDiv = document.getElementById('swipe-success');
    const failDiv    = document.getElementById('swipe-fail');
    if (!idleDiv || !successDiv || !failDiv) return;

    idleDiv.classList.add('hidden');
    successDiv.classList.add('hidden');
    failDiv.classList.add('hidden');

    const isSuccess    = status === 200 && data.status === 'SUCCESS';
    const directionText = payload?.direction === 'IN' ? '進入' : '離開';

    if (isSuccess) {
        successDiv.classList.remove('hidden');
        document.getElementById('swipe-success-msg').textContent =
            `${directionText} ${payload?.site_id} 【${payload.gate_id}】 通行放行`;
        const oldC = successDiv.querySelector('.checkmark-circle');
        const newC = oldC.cloneNode(true);
        oldC.parentNode.replaceChild(newC, oldC);
    } else {
        failDiv.classList.remove('hidden');
        document.getElementById('swipe-fail-msg').textContent =
            `${directionText} ${payload?.site_id} 【${payload.gate_id}】 拒絕（${data.reason || '未授權'}）`;
        const oldC = failDiv.querySelector('.cross-circle');
        const newC = oldC.cloneNode(true);
        oldC.parentNode.replaceChild(newC, oldC);
    }
}

// ── Settings ──────────────────────────────────
function saveSettings() {
    const apiUrl    = document.getElementById('api-url').value.trim();
    const reportUrl = document.getElementById('report-url').value.trim();
    state.apiUrl    = apiUrl;
    state.reportUrl = reportUrl;
    localStorage.setItem('apiUrl',    apiUrl);
    localStorage.setItem('reportUrl', reportUrl);
    alert('設定已儲存');
}

function restoreSettings() {
    const apiEl    = document.getElementById('api-url');
    const reportEl = document.getElementById('report-url');
    if (apiEl)    apiEl.value    = state.apiUrl;
    if (reportEl) reportEl.value = state.reportUrl;
}

// ── Connection test ───────────────────────────
async function testServerConnection() {
    const connectionResult  = document.getElementById('connection-result');
    const connectionContent = document.getElementById('connection-content');
    if (!connectionResult || !connectionContent) return;

    connectionResult.style.display = 'block';
    connectionResult.classList.remove('success', 'error');
    connectionContent.textContent = '連線測試中…';

    try {
        const accessTest  = await fetch(`${getApiUrl()}/healthz`,    { method: 'GET' }).then(r => r.ok).catch(() => false);
        const reportTest  = await fetch(`${getReportUrl()}/healthz`,  { method: 'GET' }).then(r => r.ok).catch(() => false);
        const accessStatus = accessTest ? '✓ 連線成功' : '✗ 無法連接';
        const reportStatus = reportTest ? '✓ 連線成功' : '✗ 無法連接';
        connectionResult.classList.add(accessTest && reportTest ? 'success' : 'error');
        connectionContent.innerHTML = `
            <strong>Access API (Port 8080):</strong> ${accessStatus}<br>
            <strong>Reporting API (Port 8081):</strong> ${reportStatus}<br>
            <small style="color:var(--muted);">透過 Nginx 反向代理連接後端微服務</small>
        `;
        updateServerStatus(accessTest && reportTest);
    } catch (error) {
        connectionResult.classList.add('error');
        connectionContent.innerHTML = `<strong>連線測試失敗:</strong> ${error.message}`;
        updateServerStatus(false);
    }
}

function updateServerStatus(online) {
    const indicator = document.getElementById('status-indicator');
    const statusText = document.getElementById('status-text');
    if (!indicator || !statusText) return;
    indicator.classList.remove('online', 'offline');
    indicator.classList.add(online ? 'online' : 'offline');
    statusText.textContent = online ? '線上' : '離線';
}

// ── Period / Year selects ─────────────────────
function initYearSelects() {
    const thisYear = new Date().getFullYear();
    [document.getElementById('attendance-quarter-year'), document.getElementById('attendance-year')].forEach(sel => {
        if (!sel) return;
        sel.innerHTML = '';
        for (let y = thisYear; y >= thisYear - 5; y--) {
            const opt = document.createElement('option');
            opt.value = y; opt.textContent = y + ' 年';
            sel.appendChild(opt);
        }
    });
}

function selectPeriod(e) {
    const btn = e.target.closest('.period-btn');
    if (!btn) return;
    document.querySelectorAll('.period-btn').forEach(b => b.classList.remove('active'));
    btn.classList.add('active');
    const period = btn.dataset.period;
    document.getElementById('picker-day').style.display     = period === 'day'     ? '' : 'none';
    document.getElementById('picker-month').style.display   = period === 'month'   ? '' : 'none';
    document.getElementById('picker-quarter').style.display = period === 'quarter' ? '' : 'none';
    document.getElementById('picker-year').style.display    = period === 'year'    ? '' : 'none';
    const orgTrendBtn = document.getElementById('btn-org-trend');
    if (orgTrendBtn) orgTrendBtn.style.display = 'none';
}

function getPeriodDateRange() {
    const period = document.querySelector('.period-btn.active')?.dataset.period || 'day';
    let startDate = null, endDate = null;
    if (period === 'day') {
        const d = document.getElementById('attendance-date-day')?.value;
        startDate = endDate = d || null;
    } else if (period === 'month') {
        const m = document.getElementById('attendance-date-month')?.value;
        if (m) {
            const [y, mo] = m.split('-').map(Number);
            const last = new Date(y, mo, 0).getDate();
            startDate = `${m}-01`;
            endDate   = `${m}-${String(last).padStart(2,'0')}`;
        }
    } else if (period === 'quarter') {
        const y = document.getElementById('attendance-quarter-year')?.value;
        const q = parseInt(document.getElementById('attendance-quarter-q')?.value || '1');
        if (y) {
            const sm = (q-1)*3+1, em = q*3;
            const last = new Date(parseInt(y), em, 0).getDate();
            startDate = `${y}-${String(sm).padStart(2,'0')}-01`;
            endDate   = `${y}-${String(em).padStart(2,'0')}-${last}`;
        }
    } else if (period === 'year') {
        const y = document.getElementById('attendance-year')?.value;
        if (y) { startDate = `${y}-01-01`; endDate = `${y}-12-31`; }
    }
    return { period, startDate, endDate };
}

// ── Attendance ────────────────────────────────
async function fetchAttendance() {
    const employeeId = document.getElementById('attendance-employee-id')?.value?.trim();
    const mode = document.querySelector('input[name="attendance-mode"]:checked')?.value || 'self';
    const { period, startDate, endDate } = getPeriodDateRange();
    if (!employeeId) { displayAttendanceError('請輸入員工 ID'); return; }
    if (!startDate)  { const labels={day:'日期',month:'月份',quarter:'季度',year:'年份'}; displayAttendanceError(`請選擇${labels[period]||'日期'}`); return; }
    const isAggregated = (period==='month'||period==='quarter'||period==='year');
    try {
        if (mode === 'org') {
            const endpoint = isAggregated ? 'manager-team/aggregated' : 'manager-team';
            let url = `${getReportUrl()}/v1/reports/${endpoint}?as=${employeeId}`;
            if (startDate) url += `&start_date=${startDate}`;
            if (endDate)   url += `&end_date=${endDate}`;
            const response = await fetch(url);
            const data = await response.json();
            if (response.status===403) throw new Error(`${employeeId} 無主管權限`);
            if (!response.ok) throw new Error(data.error||'查詢失敗');
            const reports = isAggregated ? (data.aggregates||[]) : (data.reports||[]);
            state.currentOrgScope = data.manager_scope;
            state.lastReports = reports;
            displayAttendanceReport(reports, data.manager_scope, mode, period);
        } else {
            const endpoint = isAggregated ? 'attendance/aggregated' : 'attendance';
            let url = `${getReportUrl()}/v1/reports/${endpoint}?as=${employeeId}`;
            if (startDate) url += `&start_date=${startDate}`;
            if (endDate)   url += `&end_date=${endDate}`;
            const response = await fetch(url);
            const data = await response.json();
            if (!response.ok) throw new Error(data.error||'查詢失敗');
            const reports = Array.isArray(data)?data:[];
            state.currentOrgScope = null;
            state.lastReports = reports;
            displayAttendanceReport(reports, null, mode, period);
        }
    } catch (error) { displayAttendanceError(error.message); }
}

function buildAttendanceHeader(period, mode) {
    const thead = document.getElementById('attendance-thead');
    if (!thead) return;
    const prefix = period==='month'?'月':period==='quarter'?'季':period==='year'?'年':'';
    if (period==='day') {
        if (mode==='self') {
            thead.innerHTML=`<tr><th>員工 ID</th><th>姓名</th><th>身分</th><th>部門</th><th>最早進入</th><th>最晚離開</th></tr>`;
        } else {
            thead.innerHTML=`<tr><th>員工 ID</th><th>姓名</th><th>身分</th><th>部門</th><th>日期</th><th>最早進入</th><th>最晚離開</th><th>刷卡次</th><th>停留時數</th></tr>`;
        }
    } else {
        if (mode==='self') {
            thead.innerHTML=`<tr><th>員工 ID</th><th>姓名</th><th>身分</th><th>部門</th><th>${prefix}平均刷卡</th><th>${prefix}平均停留</th></tr>`;
        } else {
            thead.innerHTML=`<tr><th>員工 ID</th><th>姓名</th><th>身分</th><th>部門</th><th>${prefix}刷卡總數</th><th>${prefix}平均刷卡</th><th>${prefix}總停留</th><th>${prefix}平均停留</th></tr>`;
        }
    }
}

function displayAttendanceReport(reports, scope, mode, period) {
    const statsContainer = document.getElementById('attendance-stats');
    const tbody          = document.getElementById('attendance-tbody');
    const scopeBar       = document.getElementById('attendance-scope-bar');
    const scopeEl        = document.getElementById('attendance-scope');
    const orgTrendBtn    = document.getElementById('btn-org-trend');

    scopeBar.style.display = scope ? '' : 'none';
    if (scope) scopeEl.textContent = scope;
    if (orgTrendBtn) orgTrendBtn.style.display = (mode==='org'&&scope) ? '' : 'none';

    buildAttendanceHeader(period, mode);

    const colCount = mode==='self'
        ? (period==='day' ? 6 : 6)
        : (period==='day' ? 9 : 8);

    if (!reports||reports.length===0) {
        statsContainer.innerHTML='';
        tbody.innerHTML=`<tr class="empty"><td colspan="${colCount}" style="text-align:center;padding:40px;color:var(--muted);font-size:13px;">無資料</td></tr>`;
        return;
    }

    const prefix = period==='month'?'月':period==='quarter'?'季':period==='year'?'年':'';
    const isAggregated = (period==='month'||period==='quarter'||period==='year');
    const uniqueEmployees = isAggregated ? reports.length : new Set(reports.map(r=>r.employee_id)).size;
    const totalSwipes = reports.reduce((sum,r)=>sum+(isAggregated?(r.total_swipes||0):(r.swipe_count||0)),0);
    const totalStayHours = reports.reduce((sum,r)=>sum+(isAggregated?(r.total_stay_hours||0):(r.stay_hours||0)),0).toFixed(1);

    if (mode==='org') {
        statsContainer.innerHTML=`
            <div class="stat-item"><div class="stat-item-value">${uniqueEmployees}</div><div class="stat-item-label">員工數</div></div>
            <div class="stat-item"><div class="stat-item-value">${totalSwipes}</div><div class="stat-item-label">總刷卡次數</div></div>
            <div class="stat-item"><div class="stat-item-value">${totalStayHours} hr</div><div class="stat-item-label">總停留時數</div></div>
        `;
    } else {
        statsContainer.innerHTML=`
            <div class="stat-item"><div class="stat-item-value">${totalSwipes}</div><div class="stat-item-label">${prefix||''}總刷卡次數</div></div>
            <div class="stat-item"><div class="stat-item-value">${totalStayHours} hr</div><div class="stat-item-label">${prefix||''}總停留時數</div></div>
        `;
    }

    if (period==='day') {
        tbody.innerHTML = reports.map(r => {
            const identity = getRoleBadge(r);
            if (mode==='self') {
                return `<tr class="clickable-row" data-id="${r.employee_id}" data-name="${r.name||r.employee_id}" data-date="${r.work_date}" data-type="audit">
                    <td style="font-family:var(--mono);font-size:12px;">${r.employee_id}</td><td>${r.name||'-'}</td><td>${identity}</td><td>${r.org_path||'-'}</td>
                    <td style="font-family:var(--mono);">${formatTime(r.first_in)}</td><td style="font-family:var(--mono);">${formatTime(r.last_out)}</td>
                </tr>`;
            }
            return `<tr class="clickable-row" data-id="${r.employee_id}" data-name="${r.name||r.employee_id}" data-date="${r.work_date}" data-type="audit">
                <td style="font-family:var(--mono);font-size:12px;">${r.employee_id}</td><td>${r.name||'-'}</td><td>${identity}</td><td>${r.org_path||'-'}</td>
                <td style="font-family:var(--mono);font-size:12px;">${r.work_date||'-'}</td>
                <td style="font-family:var(--mono);">${formatTime(r.first_in)}</td>
                <td style="font-family:var(--mono);">${formatTime(r.last_out)}</td>
                <td><strong>${r.swipe_count}</strong></td>
                <td style="font-family:var(--mono);">${r.stay_hours?r.stay_hours.toFixed(1)+' hr':'-'}</td>
            </tr>`;
        }).join('');
    } else {
        tbody.innerHTML = reports.map(e => {
            const identity = getRoleBadge(e);
            if (mode==='self') {
                return `<tr class="clickable-row" data-id="${e.employee_id}" data-name="${e.name||e.employee_id}" data-type="trend">
                    <td style="font-family:var(--mono);font-size:12px;">${e.employee_id}</td><td>${e.name||'-'}</td><td>${identity}</td><td>${e.org_path||'-'}</td>
                    <td style="font-family:var(--mono);">${(e.avg_swipes||0).toFixed(1)}</td>
                    <td style="font-family:var(--mono);">${(e.avg_stay_hours||0).toFixed(1)} hr</td>
                </tr>`;
            }
            return `<tr class="clickable-row" data-id="${e.employee_id}" data-name="${e.name||e.employee_id}" data-type="trend">
                <td style="font-family:var(--mono);font-size:12px;">${e.employee_id}</td><td>${e.name||'-'}</td><td>${identity}</td><td>${e.org_path||'-'}</td>
                <td style="font-family:var(--mono);"><strong>${e.total_swipes||0}</strong></td>
                <td style="font-family:var(--mono);">${(e.avg_swipes||0).toFixed(1)}</td>
                <td style="font-family:var(--mono);">${(e.total_stay_hours||0).toFixed(1)} hr</td>
                <td style="font-family:var(--mono);">${(e.avg_stay_hours||0).toFixed(1)} hr</td>
            </tr>`;
        }).join('');
    }

    document.querySelectorAll('#attendance-tbody .clickable-row').forEach(row => {
        row.addEventListener('click', () => {
            const { id, name, date, type } = row.dataset;
            if (type==='audit') showDayAuditModal(id, name, date);
            else showPersonalTrendModal(id, name);
        });
    });
}

function displayAttendanceError(message) {
    const statsContainer = document.getElementById('attendance-stats');
    const tbody          = document.getElementById('attendance-tbody');
    const scopeBar       = document.getElementById('attendance-scope-bar');
    const orgTrendBtn    = document.getElementById('btn-org-trend');
    if (scopeBar)    scopeBar.style.display    = 'none';
    if (orgTrendBtn) orgTrendBtn.style.display = 'none';
    statsContainer.innerHTML = `<div class="error-inline">錯誤：${message}</div>`;
    tbody.innerHTML = '<tr class="empty"><td colspan="9" style="text-align:center;padding:32px;color:var(--muted);font-size:13px;">查詢失敗</td></tr>';
}

async function exportAttendanceExcel() {
    const { startDate } = getPeriodDateRange();
    try {
        let url = `${getReportUrl()}/v1/reports/attendance/export`;
        if (startDate) url += `?date=${startDate}`;
        const response = await fetch(url);
        if (!response.ok) throw new Error('匯出失敗');
        const blob = await response.blob();
        const downloadUrl = URL.createObjectURL(blob);
        const link = document.createElement('a');
        link.href = downloadUrl;
        link.download = `attendance-${startDate||new Date().toISOString().split('T')[0]}.xlsx`;
        link.click();
        URL.revokeObjectURL(downloadUrl);
    } catch (error) { alert('匯出失敗: '+error.message); }
}

// ── Modal helpers ─────────────────────────────
function openModal(title) {
    if (state.modalChart) { state.modalChart.destroy(); state.modalChart=null; }
    document.getElementById('trend-modal-title').textContent = title;
    document.getElementById('trend-modal-body').innerHTML =
        `<div id="trend-modal-loading" class="modal-loading">載入中…</div>`;
    document.getElementById('trend-modal').style.display = 'flex';
}

function setModalContent(html) {
    document.getElementById('trend-modal-body').innerHTML = html;
}

function closeTrendModal() {
    document.getElementById('trend-modal').style.display = 'none';
    if (state.modalChart) { state.modalChart.destroy(); state.modalChart=null; }
    state.modalTrendData = null;
}

// ── Chart helpers ─────────────────────────────
function chartDefaults() {
    return {
        responsive: true, maintainAspectRatio: false,
        plugins: { legend: { labels: { color: CHART_OPTS.textColor, font: { size:12 } } } },
        scales: {
            x: { ticks: { color: CHART_OPTS.mutedColor, font:{size:11} }, grid: { color: CHART_OPTS.gridColor } },
            y: { beginAtZero: true, ticks: { color: CHART_OPTS.mutedColor, font:{size:11} }, grid: { color: CHART_OPTS.gridColor } }
        }
    };
}

function buildLineChart(canvasId, labels, datasets) {
    if (state.modalChart) { state.modalChart.destroy(); state.modalChart=null; }
    const ctx = document.getElementById(canvasId);
    if (!ctx) return;
    state.modalChart = new Chart(ctx, { type:'line', data:{labels,datasets}, options: chartDefaults() });
}

// ── Day Audit Modal ───────────────────────────
async function showDayAuditModal(employeeId, name, date) {
    openModal(`${name}（${employeeId}）— ${date}`);
    try {
        const url = `${getReportUrl()}/v1/audit?badge_id=${employeeId}&start_date=${date}&end_date=${date}`;
        const response = await fetch(url);
        const events   = await response.json();
        if (!response.ok) throw new Error(events.error||'查詢失敗');
        if (!events||events.length===0) { setModalContent('<p class="modal-loading">當日無刷卡紀錄</p>'); return; }
        const sorted = [...events].sort((a,b)=>new Date(a.timestamp)-new Date(b.timestamp));
        const header = `
            <div class="audit-header-row">
                <span>#</span><span>時間</span><span>方向</span>
                <span>廠區</span><span>閘門</span><span>狀態</span><span>備註</span>
            </div>`;
        const rows = sorted.map((e,i)=>{
            const time=formatTimeDetailed(e.timestamp);
            const dirIcon=e.direction==='IN'?'↓ 進入':'↑ 離開';
            const ok=e.status==='SUCCESS';
            const statusHtml=ok
                ? '<span class="audit-status ok">成功</span>'
                : '<span class="audit-status fail">失敗</span>';
            return `<div class="audit-event-row ${ok?'':'audit-row-fail'}">
                <span class="audit-seq">#${i+1}</span>
                <span class="audit-time">${time}</span>
                <span class="audit-dir">${dirIcon}</span>
                <span class="audit-site">${e.site_id||'-'}</span>
                <span class="audit-gate">${e.gate_id||'-'}</span>
                <span>${statusHtml}</span>
                <span class="audit-reason">${e.reason||'-'}</span>
            </div>`;
        }).join('');
        setModalContent(`<div class="audit-trail-container">${header}${rows}</div>`);
    } catch (err) {
        setModalContent(`<div class="error-inline">錯誤：${err.message}</div>`);
    }
}

// ── Personal Trend Modal ──────────────────────
function showPersonalTrendModal(employeeId, name) {
    openModal(`${name}（${employeeId}）出勤趨勢`);
    const { period, startDate, endDate } = getPeriodDateRange();
    const chartHtml = `
        <div class="metric-control">
            <label>指標</label>
            <select id="personal-trend-metric" class="metric-select" onchange="reRenderPersonalChart()">
                <option value="swipe_count">刷卡次數</option>
                <option value="stay_hours">停留時數 (hrs)</option>
            </select>
        </div>
        <div class="chart-container modal-chart-320"><canvas id="modal-personal-chart"></canvas></div>
    `;
    if (period==='day') {
        const dailyData=(state.lastReports||[]).filter(r=>r.employee_id===employeeId).sort((a,b)=>a.work_date.localeCompare(b.work_date));
        if (!dailyData.length) { setModalContent('<p class="modal-loading">無資料</p>'); return; }
        state.modalPersonalData=dailyData;
        setModalContent(chartHtml);
        requestAnimationFrame(()=>reRenderPersonalChart());
    } else {
        const url=`${getReportUrl()}/v1/reports/attendance?as=${employeeId}&start_date=${startDate}&end_date=${endDate}`;
        fetch(url).then(r=>r.json()).then(data=>{
            const dailyData=(Array.isArray(data)?data:[]).filter(r=>r.employee_id===employeeId).sort((a,b)=>a.work_date.localeCompare(b.work_date));
            if (!dailyData.length) { setModalContent('<p class="modal-loading">無資料</p>'); return; }
            state.modalPersonalData=dailyData;
            setModalContent(chartHtml);
            requestAnimationFrame(()=>reRenderPersonalChart());
        }).catch(err=>setModalContent(`<div class="error-inline">錯誤：${err.message}</div>`));
    }
}

function reRenderPersonalChart() {
    const dailyData = state.modalPersonalData;
    if (!dailyData||!dailyData.length) return;
    const metric = document.getElementById('personal-trend-metric')?.value||'swipe_count';
    const metricCfg = {
        swipe_count: { label:'刷卡次數',      yTitle:'次數',       color:'#0a0a0b' },
        stay_hours:  { label:'停留時數 (hrs)', yTitle:'時數 (hrs)', color:'#2563eb' },
    };
    const cfg = metricCfg[metric];
    if (state.modalChart) { state.modalChart.destroy(); state.modalChart=null; }
    const ctx = document.getElementById('modal-personal-chart');
    if (!ctx) return;
    const labels = dailyData.map(r=>r.work_date);
    const data   = dailyData.map(r=>metric==='swipe_count'?(r.swipe_count||0):parseFloat((r.stay_hours||0).toFixed(2)));
    state.modalChart = new Chart(ctx, {
        type: 'line',
        data: { labels, datasets: [{ label:cfg.label, data, borderColor:cfg.color, backgroundColor:cfg.color+'18', tension:.4, fill:true, pointRadius:3, borderWidth:1.5 }] },
        options: {
            ...chartDefaults(),
            scales: {
                ...chartDefaults().scales,
                y: { ...chartDefaults().scales.y, title:{ display:true, text:cfg.yTitle, color:CHART_OPTS.mutedColor, font:{size:11} } }
            }
        }
    });
}

// ── Org Trend ─────────────────────────────────
async function showOrgTrend() {
    const scope = state.currentOrgScope;
    if (!scope) return;
    const managerId = document.getElementById('attendance-employee-id')?.value?.trim();
    const { period, startDate, endDate } = getPeriodDateRange();
    openModal(`底下組織趨勢 — ${scope}`);
    const orgSize = new Set((state.lastReports||[]).map(r=>r.employee_id)).size||1;
    state.modalOrgPeriod = period;
    if (period==='day') {
        const reports = state.lastReports||[];
        state.modalTrendData = { type:'day', reports, orgSize };
        const byDate={};
        for (const r of reports) {
            if (!byDate[r.work_date]) byDate[r.work_date]={ swipes:0, persons:new Set(), stay:0 };
            byDate[r.work_date].swipes+=r.swipe_count||0;
            byDate[r.work_date].persons.add(r.employee_id);
            byDate[r.work_date].stay+=r.stay_hours||0;
        }
        const d=Object.values(byDate)[0]||{ swipes:0, persons:new Set(), stay:0 };
        setModalContent(`
            <div class="stats-grid" style="border:none;padding:0;background:transparent;">
                <div class="stat-item"><div class="stat-item-value">${(d.swipes/orgSize).toFixed(2)}</div><div class="stat-item-label">平均刷卡次數</div></div>
                <div class="stat-item"><div class="stat-item-value">${d.persons.size}</div><div class="stat-item-label">出勤人數</div></div>
                <div class="stat-item"><div class="stat-item-value">${(d.stay/orgSize).toFixed(2)}</div><div class="stat-item-label">平均停留時數</div></div>
            </div>
        `);
    } else {
        const periodLabel = period==='quarter'?'季':period==='year'?'年':'月';
        const trendPeriod = period==='year'?'year':'day';
        try {
            let url=`${getReportUrl()}/v1/reports/trend?period=${trendPeriod}&as=${managerId}`;
            if (startDate) url+=`&start_date=${startDate}`;
            if (endDate)   url+=`&end_date=${endDate}`;
            const response=await fetch(url, { headers:state.token?{Authorization:`Bearer ${state.token}`}:{} });
            const data=await response.json();
            if (!response.ok) throw new Error(data.error||'趨勢查詢失敗');
            const trends=(data.trends||[]).sort((a,b)=>a.bucket.localeCompare(b.bucket));
            const summary=data.summary||{};
            state.modalTrendData={ type:'trend', trends };
            setModalContent(`
                <div class="stats-grid" style="border:none;padding:0 0 20px;background:transparent;">
                    <div class="stat-item"><div class="stat-item-value">${(summary.avg_swipes_per_person||0).toFixed(2)}</div><div class="stat-item-label">${periodLabel}平均刷卡次數</div></div>
                    <div class="stat-item"><div class="stat-item-value">${(summary.avg_head_count||0).toFixed(1)}</div><div class="stat-item-label">${periodLabel}平均出勤人數</div></div>
                    <div class="stat-item"><div class="stat-item-value">${(summary.avg_stay_hrs||0).toFixed(2)}</div><div class="stat-item-label">${periodLabel}平均停留時數</div></div>
                </div>
                <div class="metric-control">
                    <label>指標</label>
                    <select id="org-trend-metric" class="metric-select" onchange="reRenderOrgChart()">
                        <option value="avg_swipe">平均刷卡次數</option>
                        <option value="persons">出勤人數</option>
                        <option value="avg_stay">平均停留時數</option>
                    </select>
                </div>
                <div class="chart-container modal-chart-300"><canvas id="modal-org-chart"></canvas></div>
            `);
            requestAnimationFrame(()=>reRenderOrgChart());
        } catch (err) {
            setModalContent(`<div class="error-inline">錯誤：${err.message}</div>`);
        }
    }
}

function reRenderOrgChart() {
    const td=state.modalTrendData;
    if (!td) return;
    const metric=document.getElementById('org-trend-metric')?.value||'avg_swipe';
    const metricCfg={
        avg_swipe:{ label:'平均刷卡次數',      yTitle:'次數',       color:'#0a0a0b' },
        persons:  { label:'出勤人數',           yTitle:'人數',       color:'#2563eb' },
        avg_stay: { label:'平均停留時數 (hrs)', yTitle:'時數 (hrs)', color:'#16a34a' },
    };
    const cfg=metricCfg[metric]||metricCfg.avg_swipe;
    if (state.modalChart) { state.modalChart.destroy(); state.modalChart=null; }
    const ctx=document.getElementById('modal-org-chart');
    if (!ctx) return;
    if (td.type==='day') return;
    const { trends }=td;
    const labels=trends.map(t=>t.bucket);
    const data=trends.map(t=>{
        if (metric==='avg_swipe') return t.head_count>0?parseFloat(((t.total_swipes||0)/t.head_count).toFixed(2)):0;
        if (metric==='persons')   return t.head_count||0;
        return parseFloat((t.avg_stay_hrs||0).toFixed(2));
    });
    const opts = {
        ...chartDefaults(),
        scales: {
            ...chartDefaults().scales,
            y: { ...chartDefaults().scales.y, title:{ display:true, text:cfg.yTitle, color:CHART_OPTS.mutedColor, font:{size:11} } }
        }
    };
    state.modalChart = new Chart(ctx, {
        type:'line',
        data:{ labels, datasets:[{ label:cfg.label, data, borderColor:cfg.color, backgroundColor:cfg.color+'18', tension:.4, fill:true, pointRadius:3, borderWidth:1.5 }] },
        options: opts
    });
}

// ── Alerts ────────────────────────────────────
async function fetchAlerts() {
    const severity = document.getElementById('alert-severity')?.value;
    try {
        let url=`${getReportUrl()}/v1/alerts`;
        if (severity) url+=`?severity=${severity}`;
        const response=await fetch(url);
        const data=await response.json();
        if (!response.ok) throw new Error(data.error||'查詢失敗');
        displayAlerts(data);
    } catch (error) { alert('警報查詢失敗: '+error.message); }
}

function displayAlerts(alerts) {
    const container=document.getElementById('alerts-list');
    if (!container) return;
    if (!alerts||alerts.length===0) { container.innerHTML='<p class="placeholder">暫無警報紀錄</p>'; return; }
    const severityLabel={'CRITICAL':'嚴重','HIGH':'高','MEDIUM':'中','LOW':'低'};
    container.innerHTML = alerts.map(alert=>{
        const sc=alert.severity.toLowerCase();
        return `
            <div class="alert-item ${sc}">
                <div class="alert-header">
                    <span class="alert-type">${alert.alert_type}</span>
                    <span class="alert-time">${new Date(alert.occurred_at).toLocaleString('zh-TW')}</span>
                </div>
                <div class="alert-details">
                    嚴重程度：<strong>${severityLabel[alert.severity]||alert.severity}</strong> ·
                    員工 ID：<strong>${alert.badge_id||'—'}</strong> ·
                    地點：<strong>${alert.site_id}/${alert.gate_id}</strong>
                </div>
            </div>
        `;
    }).join('');
}

// ── Dev Login ─────────────────────────────────
async function devLogin() {
    const badge=document.getElementById('dev-login-badge')?.value?.trim();
    if (!badge) { alert('請輸入員工ID'); return; }
    try {
        const response=await fetch(`${getReportUrl()}/v1/dev/login`,{
            method:'POST', headers:{'Content-Type':'application/json'},
            body:JSON.stringify({ badge_id:badge })
        });
        const data=await response.json();
        if (!response.ok) throw new Error(data.error||'登入失敗');
        state.token=data.access_token;
        state.currentBadge=badge;
        localStorage.setItem('pacs_token',data.access_token);
        localStorage.setItem('current_badge',badge);
        const tokenInfo=document.getElementById('token-info');
        if (tokenInfo) {
            tokenInfo.style.display='block';
            tokenInfo.innerHTML=`
                登入成功 — ${badge}<br>
                Token: ${data.access_token.substring(0,60)}…<br>
                有效期: ${Math.floor(data.expires_in/3600)} 小時
            `;
        }
        updateProfileDisplay(badge);
    } catch (error) { alert('登入失敗: '+error.message); }
}

function updateProfileDisplay(badge) {
    const profileName=document.getElementById('profile-name');
    if (profileName) profileName.textContent=badge;
}

// ── Utilities ─────────────────────────────────
function formatTime(timeString) {
    if (!timeString) return '-';
    try { return new Date(timeString).toLocaleTimeString('zh-TW',{hour:'2-digit',minute:'2-digit'}); }
    catch { return timeString; }
}

function formatTimeDetailed(timeString) {
    if (!timeString) return '-';
    try { return new Date(timeString).toLocaleTimeString('zh-TW',{hour:'2-digit',minute:'2-digit',second:'2-digit'}); }
    catch { return timeString; }
}

function getDateDaysAgo(days) {
    const date=new Date();
    date.setDate(date.getDate()-days);
    return date.toISOString().split('T')[0];
}

window.addEventListener('load',()=>{
    const today=new Date().toISOString().split('T')[0];
    const el=document.getElementById('attendance-date-day');
    if (el) el.value=today;
});

setInterval(()=>{
    fetch(`${getApiUrl()}/healthz`,{method:'GET'})
        .then(r=>updateServerStatus(r.ok))
        .catch(()=>updateServerStatus(false));
}, 30000);