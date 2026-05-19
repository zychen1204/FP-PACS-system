// ============================================
// PACS Frontend - Modern Application
// ============================================

// State Management
const state = {
    // API URLs
    apiUrl: localStorage.getItem('apiUrl') || 'http://localhost:8080',
    reportUrl: localStorage.getItem('reportUrl') || 'http://localhost:8081',
    
    // Auth
    token: localStorage.getItem('pacs_token') || null,
    currentBadge: localStorage.getItem('current_badge') || 'B001',
    
    // Data
    swipeHistory: JSON.parse(localStorage.getItem('swipeHistory')) || [],
    trendChart: null,
    modalChart: null,
    modalTrendData: null,
    modalBadge: null,
    modalScope: null,
    
    // UI State
    selectedTier: 'outer',
    selectedDirection: 'IN',
    
    // Server status
    serverOnline: false
};

// Helper Functions
function getRoleBadge(report) {
    const status = report.status || (report.job_level) || (report.is_manager ? 'MANAGER_L2' : 'STAFF');
    const roles = {
        'MANAGER_L1': { label: '🎖️ 一級主管', class: 'mgr-1' },
        'MANAGER_L2': { label: '👔 二級主管', class: 'mgr-2' },
        'mgr-1': { label: '🎖️ 一級主管', class: 'mgr-1' },
        'mgr-2': { label: '👔 二級主管', class: 'mgr-2' },
        'STAFF': { label: '👤 員工', class: 'employee' },
        'employee': { label: '👤 員工', class: 'employee' }
    };
    const role = roles[status] || roles['STAFF'];
    return `<span class="badge-role ${role.class}">${role.label}</span>`;
}

// ============ INITIALIZATION ============
document.addEventListener('DOMContentLoaded', () => {
    initializeApp();
});

function initializeApp() {
    setupEventListeners();
    restoreSettings();
    testServerConnection();
    loadSwipeHistory();
}

// ============ EVENT LISTENERS ============
function setupEventListeners() {
    // New navigation system
    document.querySelectorAll('.nav-item').forEach(item => {
        item.addEventListener('click', switchTab);
    });

    // Gate tier selection
    document.querySelectorAll('.tier-btn').forEach(btn => {
        btn.addEventListener('click', selectTier);
    });

    // Direction selection
    document.querySelectorAll('.direction-btn').forEach(btn => {
        btn.addEventListener('click', selectDirection);
    });

    // Swipe Tab
    document.getElementById('btn-swipe')?.addEventListener('click', sendSwipe);
    document.getElementById('btn-clear-history')?.addEventListener('click', clearHistory);

    // Attendance Tab
    document.getElementById('btn-fetch-attendance')?.addEventListener('click', fetchAttendance);
    document.getElementById('btn-export-attendance')?.addEventListener('click', exportAttendanceExcel);
    document.getElementById('btn-org-trend')?.addEventListener('click', showOrgTrend);
    document.querySelectorAll('.period-btn').forEach(btn => {
        btn.addEventListener('click', selectPeriod);
    });

    // Trend Modal
    document.getElementById('btn-close-trend-modal')?.addEventListener('click', closeTrendModal);
    document.getElementById('trend-modal')?.addEventListener('click', e => {
        if (e.target.id === 'trend-modal') closeTrendModal();
    });
    document.getElementById('trend-modal-metric')?.addEventListener('change', () => {
        if (state.modalTrendData) renderModalChart(state.modalTrendData);
    });

    initQuarterYearSelect();

    // Alerts Tab
    document.getElementById('btn-fetch-alerts')?.addEventListener('click', fetchAlerts);

    // Settings Tab
    document.getElementById('btn-save-settings')?.addEventListener('click', saveSettings);
    document.getElementById('btn-test-connection')?.addEventListener('click', testServerConnection);
    document.getElementById('btn-dev-login')?.addEventListener('click', devLogin);
}

// ============ TAB SWITCHING ============
function switchTab(e) {
    e.preventDefault();
    const tabId = e.target.closest('.nav-item')?.getAttribute('data-tab');
    if (!tabId) return;
    
    // Update active nav
    document.querySelectorAll('.nav-item').forEach(item => {
        item.classList.remove('active');
    });
    e.target.closest('.nav-item').classList.add('active');
    
    // Update active tab content
    document.querySelectorAll('.tab-content').forEach(tab => {
        tab.classList.remove('active');
    });
    document.getElementById(tabId)?.classList.add('active');
    
    // Update page title
    const titles = {
        'swipe-tab': '刷卡模擬器',
        'attendance-tab': '出席報表',
        'alerts-tab': '警報異常',
        'settings-tab': '系統設定'
    };
    
    const pageTitle = document.getElementById('page-title');
    if (pageTitle) pageTitle.textContent = titles[tabId] || '刷卡模擬器';
}

// ============ GATE TIER SELECTION ============
function selectTier(e) {
    e.preventDefault();
    const tier = e.target.closest('.tier-btn')?.getAttribute('data-tier');
    if (!tier) return;
    
    document.querySelectorAll('.tier-btn').forEach(btn => {
        btn.classList.remove('active');
    });
    e.target.closest('.tier-btn').classList.add('active');
    state.selectedTier = tier;
    
    // Update gate options
    const gateSelect = document.getElementById('gate-id');
    if (gateSelect) {
        if (tier === 'outer') {
            gateSelect.innerHTML = `
                <option value="Gate-1A">Gate-1A (外層)</option>
                <option value="Gate-1B">Gate-1B (外層)</option>
                <option value="Gate-1C">Gate-1C (外層)</option>
            `;
        } else {
            gateSelect.innerHTML = `
                <option value="Gate-2A">Gate-2A (內層)</option>
                <option value="Gate-2B">Gate-2B (內層)</option>
                <option value="Gate-2C">Gate-2C (內層)</option>
            `;
        }
    }
}

// ============ DIRECTION SELECTION ============
function selectDirection(e) {
    e.preventDefault();
    const direction = e.target.closest('.direction-btn')?.getAttribute('data-direction');
    if (!direction) return;
    
    document.querySelectorAll('.direction-btn').forEach(btn => {
        btn.classList.remove('active');
    });
    e.target.closest('.direction-btn').classList.add('active');
    state.selectedDirection = direction;
}

// ============ API URL HELPERS ============
function getApiUrl() {
    return state.apiUrl || window.location.origin;
}
function getReportUrl() {
    return state.reportUrl || window.location.origin;
}

// ============ SWIPE REQUEST ============
async function sendSwipe() {
    const badgeId = document.getElementById('badge-id')?.value?.trim();
    const siteId = document.getElementById('site-id')?.value || 'FAB12-A';
    const gateId = document.getElementById('gate-id')?.value;
    
    if (!badgeId) {
        alert('請輸入員工證件 ID');
        return;
    }
    
    const payload = {
        badge_id: badgeId,
        site_id: siteId,
        gate_id: gateId,
        direction: state.selectedDirection
    };
    
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

// ============ SWIPE RESPONSE DISPLAY ============
function displaySwipeResponse(status, data, payload) {
    const idleDiv = document.getElementById('swipe-idle');
    const successDiv = document.getElementById('swipe-success');
    const failDiv = document.getElementById('swipe-fail');
    
    if (!idleDiv || !successDiv || !failDiv) return;
    
    idleDiv.classList.add('hidden');
    successDiv.classList.add('hidden');
    failDiv.classList.add('hidden');
    
    const isSuccess = status === 200 && data.status === 'SUCCESS';
    const directionText = payload?.direction === 'IN' ? '進入' : '離開';
    const gateText = payload?.gate_id ? ` (${payload.gate_id})` : '';
    
    if (isSuccess) {
        successDiv.classList.remove('hidden');
        const siteText = payload?.site_id ? `${payload.site_id} ` : '';
        document.getElementById('swipe-success-msg').textContent = `${directionText} ${siteText}【${payload.gate_id}】刷卡成功`;
        
        // Reset animation by removing and re-adding elements
        const oldCircle = successDiv.querySelector('.checkmark-circle');
        const newCircle = oldCircle.cloneNode(true);
        oldCircle.parentNode.replaceChild(newCircle, oldCircle);
    } else {
        failDiv.classList.remove('hidden');
        const siteText = payload?.site_id ? `${payload.site_id} ` : '';
        const reason = data.reason ? `${data.reason} - ` : '';
        document.getElementById('swipe-fail-msg').textContent = `${directionText} ${siteText}【${payload.gate_id}】刷卡失敗 (${data.reason || '拒絕通行'})`;
        
        // Reset animation by removing and re-adding elements
        const oldCircle = failDiv.querySelector('.cross-circle');
        const newCircle = oldCircle.cloneNode(true);
        oldCircle.parentNode.replaceChild(newCircle, oldCircle);
    }
}



// ============ REPORT FETCHING ============
async function fetchReport() {
    try {
        const response = await fetch(`${getReportUrl()}/v1/reports/attendance`, {
            method: 'GET',
            headers: { 'Content-Type': 'application/json' }
        });

        const data = await response.json();

        if (!response.ok) {
            throw new Error(data.error || 'Failed to fetch report');
        }

        displayReport(data);

    } catch (error) {
        displayReportError(error.message);
    }
}

// ============ REPORT DISPLAY ============
function displayReport(reports) {
    const responseBox = document.getElementById('report-response');
    const tbody = document.getElementById('report-tbody');
    const statsBox = document.getElementById('report-stats');

    responseBox.classList.remove('hidden', 'error');
    responseBox.classList.add('success');

    if (!reports || reports.length === 0) {
        tbody.innerHTML = '<tr><td colspan="8" class="empty">沒有找到出席紀錄</td></tr>';
        statsBox.innerHTML = '<div class="stat-item"><div class="stat-item-value">0</div><div class="stat-item-label">總紀錄數</div></div>';
        return;
    }

    const totalRecords = reports.length;
    const totalSwipes = reports.reduce((sum, r) => sum + r.swipe_count, 0);
    const uniqueEmployees = new Set(reports.map(r => r.employee_id)).size;

    statsBox.innerHTML = `
        <div class="stat-item">
            <div class="stat-item-value">${totalRecords}</div>
            <div class="stat-item-label">總紀錄數</div>
        </div>
        <div class="stat-item">
            <div class="stat-item-value">${uniqueEmployees}</div>
            <div class="stat-item-label">員工人數</div>
        </div>
        <div class="stat-item">
            <div class="stat-item-value">${totalSwipes}</div>
            <div class="stat-item-label">總刷卡次數</div>
        </div>
    `;

    tbody.innerHTML = reports.map(report => `
        <tr>
            <td>${report.employee_id}</td>
            <td>${report.name || '-'}</td>
            <td>${report.org_path || '-'}</td>
            <td>${report.work_date || '-'}</td>
            <td>${formatTime(report.first_in)}</td>
            <td>${formatTime(report.last_out)}</td>
            <td><strong>${report.swipe_count}</strong></td>
            <td>${report.stay_hours ? report.stay_hours.toFixed(1) + ' hr' : '-'}</td>
        </tr>
    `).join('');

    responseBox.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
}

function displayReportError(message) {
    const responseBox = document.getElementById('report-response');
    const tbody = document.getElementById('report-tbody');
    const statsBox = document.getElementById('report-stats');

    responseBox.classList.remove('hidden', 'success');
    responseBox.classList.add('error');

    statsBox.innerHTML = `<div style="color: var(--danger); padding: 15px;">❌ 錯誤: ${message}</div>`;
    tbody.innerHTML = '<tr><td colspan="8" class="empty">無法載入報表</td></tr>';

    responseBox.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
}

// ============ SETTINGS ============
function saveSettings() {
    const apiUrl = document.getElementById('api-url').value.trim();
    const reportUrl = document.getElementById('report-url').value.trim();

    state.apiUrl = apiUrl;
    state.reportUrl = reportUrl;

    localStorage.setItem('apiUrl', apiUrl);
    localStorage.setItem('reportUrl', reportUrl);

    alert('設定已儲存');
}

function restoreSettings() {
    document.getElementById('api-url').value = state.apiUrl;
    document.getElementById('report-url').value = state.reportUrl;
}

// ============ CONNECTION TEST ============
async function testServerConnection() {
    const connectionResult = document.getElementById('connection-result');
    const connectionContent = document.getElementById('connection-content');

    connectionResult.classList.remove('hidden', 'success', 'error');

    try {
        const accessTest = await fetch(`${getApiUrl()}/api/healthz`, { method: 'GET' })
            .then(r => r.ok).catch(() => false);
        const reportTest = await fetch(`${getReportUrl()}/api/report-healthz`, { method: 'GET' })
            .then(r => r.ok).catch(() => false);

        const accessStatus = accessTest ? '✓ 連線成功' : '✗ 無法連接';
        const reportStatus = reportTest ? '✓ 連線成功' : '✗ 無法連接';

        connectionResult.classList.add(accessTest && reportTest ? 'success' : 'error');

        connectionContent.innerHTML = `
            <strong>Access API (Port 8080):</strong> ${accessStatus}<br>
            <strong>Reporting API (Port 8081):</strong> ${reportStatus}<br><br>
            <small>透過 Nginx 反向代理連接後端微服務</small>
        `;

        updateServerStatus(accessTest && reportTest);

    } catch (error) {
        connectionResult.classList.add('error');
        connectionContent.innerHTML = `<strong>❌ 連線測試失敗:</strong> ${error.message}`;
        updateServerStatus(false);
    }
}

function updateServerStatus(online) {
    const indicator = document.getElementById('status-indicator');
    const statusText = document.getElementById('status-text');

    indicator.classList.remove('online', 'offline');
    indicator.classList.add(online ? 'online' : 'offline');
    statusText.textContent = online ? '線上' : '離線';
}

// ============ PERIOD SELECTOR ============
function initQuarterYearSelect() {
    const sel = document.getElementById('attendance-quarter-year');
    if (!sel) return;
    const thisYear = new Date().getFullYear();
    for (let y = thisYear; y >= thisYear - 5; y--) {
        const opt = document.createElement('option');
        opt.value = y;
        opt.textContent = y + ' 年';
        sel.appendChild(opt);
    }
}

function selectPeriod(e) {
    const btn = e.target.closest('.period-btn');
    if (!btn) return;
    document.querySelectorAll('.period-btn').forEach(b => b.classList.remove('active'));
    btn.classList.add('active');
    const period = btn.dataset.period;
    document.getElementById('picker-day').style.display = period === 'day' ? '' : 'none';
    document.getElementById('picker-month').style.display = period === 'month' ? '' : 'none';
    document.getElementById('picker-quarter').style.display = period === 'quarter' ? '' : 'none';
}

function getPeriodDateRange() {
    const period = document.querySelector('.period-btn.active')?.dataset.period || 'day';
    let startDate = null, endDate = null;

    if (period === 'day') {
        const d = document.getElementById('attendance-date-day')?.value;
        startDate = d || null;
        endDate = d || null;
    } else if (period === 'month') {
        const m = document.getElementById('attendance-date-month')?.value;
        if (m) {
            const [y, mo] = m.split('-').map(Number);
            const last = new Date(y, mo, 0).getDate();
            startDate = `${m}-01`;
            endDate = `${m}-${String(last).padStart(2, '0')}`;
        }
    } else if (period === 'quarter') {
        const y = document.getElementById('attendance-quarter-year')?.value;
        const q = parseInt(document.getElementById('attendance-quarter-q')?.value || '1');
        if (y) {
            const sm = (q - 1) * 3 + 1;
            const em = q * 3;
            const last = new Date(parseInt(y), em, 0).getDate();
            startDate = `${y}-${String(sm).padStart(2, '0')}-01`;
            endDate = `${y}-${String(em).padStart(2, '0')}-${last}`;
        }
    }
    return { period, startDate, endDate };
}

// ============ ATTENDANCE REPORT ============
async function fetchAttendance() {
    const employeeId = document.getElementById('attendance-employee-id')?.value?.trim();
    const mode = document.querySelector('input[name="attendance-mode"]:checked')?.value || 'self';
    const { period, startDate, endDate } = getPeriodDateRange();

    if (!employeeId) {
        displayAttendanceError('請輸入員工 ID');
        return;
    }

    try {
        if (mode === 'org') {
            let url = `${getReportUrl()}/v1/reports/manager-team?as=${employeeId}`;
            if (startDate) url += `&date=${startDate}`;

            const response = await fetch(url);
            const data = await response.json();

            if (response.status === 403) throw new Error(`${employeeId} 無主管權限，無法查詢底下組織`);
            if (!response.ok) throw new Error(data.error || '查詢失敗');

            state.currentOrgScope = data.manager_scope;
            displayAttendanceReport(data.reports || [], data.manager_scope, mode);
        } else {
            let url = `${getReportUrl()}/v1/reports/attendance?as=${state.currentBadge}`;
            if (startDate) url += `&date=${startDate}`;

            const response = await fetch(url);
            const data = await response.json();

            if (!response.ok) throw new Error(data.error || '查詢失敗');

            let filtered = data.filter(r => r.employee_id === employeeId);
            if (period !== 'day' && startDate && endDate) {
                filtered = filtered.filter(r => r.work_date >= startDate && r.work_date <= endDate);
            }
            state.currentOrgScope = null;
            displayAttendanceReport(filtered, null, mode);
        }
    } catch (error) {
        displayAttendanceError(error.message);
    }
}

function displayAttendanceReport(reports, scope, mode) {
    const statsContainer = document.getElementById('attendance-stats');
    const tbody = document.getElementById('attendance-tbody');
    const scopeBar = document.getElementById('attendance-scope-bar');
    const scopeEl = document.getElementById('attendance-scope');
    const orgTrendBtn = document.getElementById('btn-org-trend');

    scopeBar.style.display = scope ? '' : 'none';
    if (scope) scopeEl.textContent = scope;
    if (orgTrendBtn) orgTrendBtn.style.display = (mode === 'org' && scope) ? '' : 'none';

    if (!reports || reports.length === 0) {
        statsContainer.innerHTML = '<p class="placeholder">無資料</p>';
        tbody.innerHTML = '<tr class="empty"><td colspan="9">無結果</td></tr>';
        return;
    }

    const uniqueEmployees = new Set(reports.map(r => r.employee_id)).size;
    const totalSwipes = reports.reduce((sum, r) => sum + (r.swipe_count || 0), 0);
    const avgStayHours = (reports.reduce((sum, r) => sum + (r.stay_hours || 0), 0) / reports.length).toFixed(1);

    statsContainer.innerHTML = `
        <div class="stat-item"><div class="stat-item-value">${reports.length}</div><div class="stat-item-label">總紀錄</div></div>
        <div class="stat-item"><div class="stat-item-value">${uniqueEmployees}</div><div class="stat-item-label">員工數</div></div>
        <div class="stat-item"><div class="stat-item-value">${totalSwipes}</div><div class="stat-item-label">刷卡次</div></div>
        <div class="stat-item"><div class="stat-item-value">${avgStayHours}</div><div class="stat-item-label">平均停留</div></div>
    `;

    tbody.innerHTML = reports.map(report => {
        const identity = getRoleBadge(report);
        return `
        <tr class="clickable-row" data-id="${report.employee_id}" data-name="${report.name || report.employee_id}" style="cursor:pointer;" title="點擊查看趨勢分析">
            <td>${report.employee_id}</td>
            <td>${report.name || '-'}</td>
            <td>${identity}</td>
            <td>${report.org_path || '-'}</td>
            <td>${report.work_date || '-'}</td>
            <td>${formatTime(report.first_in)}</td>
            <td>${formatTime(report.last_out)}</td>
            <td><strong>${report.swipe_count}</strong></td>
            <td>${report.stay_hours ? report.stay_hours.toFixed(1) + ' hr' : '-'}</td>
        </tr>`;
    }).join('');

    document.querySelectorAll('#attendance-tbody .clickable-row').forEach(row => {
        row.addEventListener('click', () => {
            showEmployeeTrend(row.dataset.id, row.dataset.name);
        });
        row.addEventListener('mouseenter', () => row.style.background = 'rgba(30,64,175,0.15)');
        row.addEventListener('mouseleave', () => row.style.background = '');
    });
}

function displayAttendanceError(message) {
    const statsContainer = document.getElementById('attendance-stats');
    const tbody = document.getElementById('attendance-tbody');
    const scopeBar = document.getElementById('attendance-scope-bar');
    const orgTrendBtn = document.getElementById('btn-org-trend');

    if (scopeBar) scopeBar.style.display = 'none';
    if (orgTrendBtn) orgTrendBtn.style.display = 'none';
    statsContainer.innerHTML = `<div style="color: var(--danger);">❌ ${message}</div>`;
    tbody.innerHTML = '<tr class="empty"><td colspan="9">查詢失敗</td></tr>';
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
        link.download = `attendance-${startDate || new Date().toISOString().split('T')[0]}.xlsx`;
        link.click();
        URL.revokeObjectURL(downloadUrl);
    } catch (error) {
        alert('匯出失敗: ' + error.message);
    }
}

// ============ TREND MODAL ============
function showEmployeeTrend(employeeId, name) {
    document.getElementById('trend-modal-title').textContent = `📈 ${name}（${employeeId}）趨勢分析`;
    const scopeBar = document.getElementById('trend-modal-scope');
    if (scopeBar) scopeBar.style.display = 'none';
    state.modalBadge = employeeId;
    state.modalScope = null;
    openTrendModal();
}

function showOrgTrend() {
    const scope = state.currentOrgScope;
    if (!scope) return;
    document.getElementById('trend-modal-title').textContent = `📈 底下組織趨勢分析`;
    const scopeBar = document.getElementById('trend-modal-scope');
    const scopeText = document.getElementById('trend-modal-scope-text');
    if (scopeBar) scopeBar.style.display = '';
    if (scopeText) scopeText.textContent = scope;
    state.modalBadge = null;
    state.modalScope = scope;
    openTrendModal();
}

async function openTrendModal() {
    const modal = document.getElementById('trend-modal');
    modal.style.display = 'flex';
    document.getElementById('trend-modal-loading').style.display = 'block';
    document.getElementById('trend-modal-chart-wrap').style.display = 'none';

    const { period, startDate, endDate } = getPeriodDateRange();
    const apiPeriod = period === 'day' ? 'day' : period === 'month' ? 'month' : 'quarter';

    let url = `${getReportUrl()}/v1/reports/trend?period=${apiPeriod}`;
    if (startDate) url += `&start_date=${startDate}`;
    if (endDate) url += `&end_date=${endDate}`;
    if (state.modalBadge) url += `&as=${state.modalBadge}`;

    try {
        const response = await fetch(url);
        const data = await response.json();
        if (!response.ok) throw new Error(data.error || '查詢失敗');

        state.modalTrendData = (data.trends || []).reverse();
        document.getElementById('trend-modal-loading').style.display = 'none';
        document.getElementById('trend-modal-chart-wrap').style.display = 'block';
        renderModalChart(state.modalTrendData);
    } catch (err) {
        document.getElementById('trend-modal-loading').textContent = `❌ ${err.message}`;
    }
}

function renderModalChart(trends) {
    if (state.modalChart) {
        state.modalChart.destroy();
        state.modalChart = null;
    }
    const ctx = document.getElementById('trend-modal-chart');
    if (!ctx || !trends || trends.length === 0) return;

    const metric = document.getElementById('trend-modal-metric')?.value || 'avg_stay_hrs';
    const metricConfig = {
        avg_stay_hrs: { label: '平均停留時數 (hrs)', color: '#3b82f6', unit: '時數 (hrs)' },
        head_count:   { label: '出勤人頭數 (persons)', color: '#10b981', unit: '人數 (persons)' },
        total_swipes: { label: '總刷卡次數 (counts)', color: '#fbbf24', unit: '次數 (counts)' }
    };
    const cfg = metricConfig[metric];

    state.modalChart = new Chart(ctx, {
        type: 'line',
        data: {
            labels: trends.map(t => t.bucket),
            datasets: [{
                label: cfg.label,
                data: trends.map(t => t[metric]),
                borderColor: cfg.color,
                backgroundColor: cfg.color + '22',
                tension: 0.4,
                fill: true,
                pointRadius: 4,
                pointHoverRadius: 6
            }]
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            plugins: { legend: { labels: { color: '#f1f5f9' } } },
            scales: {
                y: {
                    beginAtZero: true,
                    title: { display: true, text: cfg.unit, color: '#94a3b8', font: { size: 12 } },
                    ticks: { color: '#f1f5f9' },
                    grid: { color: 'rgba(71,85,105,0.2)' }
                },
                x: {
                    ticks: { color: '#f1f5f9' },
                    grid: { color: 'rgba(71,85,105,0.2)' }
                }
            }
        }
    });
}

function closeTrendModal() {
    document.getElementById('trend-modal').style.display = 'none';
    if (state.modalChart) {
        state.modalChart.destroy();
        state.modalChart = null;
    }
    state.modalTrendData = null;
}

// ============ ALERTS ============
async function fetchAlerts() {
    const severity = document.getElementById('alert-severity')?.value;
    
    try {
        let url = `${getReportUrl()}/v1/alerts`;
        if (severity) {
            url += `?severity=${severity}`;
        }
        
        const response = await fetch(url);
        const data = await response.json();
        
        if (!response.ok) {
            throw new Error(data.error || '查詢失敗');
        }
        
        displayAlerts(data);
        
    } catch (error) {
        alert('警報查詢失敗: ' + error.message);
    }
}

function displayAlerts(alerts) {
    const container = document.getElementById('alerts-list');
    if (!container) return;
    
    if (!alerts || alerts.length === 0) {
        container.innerHTML = '<p class="placeholder">暫無警報</p>';
        return;
    }
    
    container.innerHTML = alerts.map(alert => {
        const severityClass = alert.severity.toLowerCase();
        const severityIcon = {
            'critical': '🔴',
            'high': '🟠',
            'medium': '🟡',
            'low': '🟢'
        }[severityClass] || '⚠️';
        
        return `
            <div class="alert-item ${severityClass}">
                <div class="alert-header">
                    <span class="alert-type">${severityIcon} ${alert.alert_type}</span>
                    <span class="alert-time">${new Date(alert.occurred_at).toLocaleString('zh-TW')}</span>
                </div>
                <div class="alert-details">
                    <div><strong>嚴重程度:</strong> ${alert.severity}</div>
                    <div><strong>員工ID:</strong> ${alert.badge_id || '-'}</div>
                    <div><strong>地點:</strong> ${alert.site_id}/${alert.gate_id}</div>
                </div>
            </div>
        `;
    }).join('');
}

// ============ DEV LOGIN ============
async function devLogin() {
    const badge = document.getElementById('dev-login-badge')?.value?.trim();
    
    if (!badge) {
        alert('請輸入員工ID');
        return;
    }
    
    try {
        const response = await fetch(`${getReportUrl()}/v1/dev/login`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ badge_id: badge })
        });
        
        const data = await response.json();
        
        if (!response.ok) {
            throw new Error(data.error || '登入失敗');
        }
        
        state.token = data.access_token;
        state.currentBadge = badge;
        
        localStorage.setItem('pacs_token', data.access_token);
        localStorage.setItem('current_badge', badge);
        
        const tokenInfo = document.getElementById('token-info');
        const roleText = data.is_manager ? "👔 主管" : "👤 員工";
        
        if (tokenInfo) {
            tokenInfo.innerHTML = `
                <strong>✓ 登入成功</strong><br>
                員工: ${badge} (${roleText})<br>
                Token: ${data.access_token.substring(0, 50)}...<br>
                有效期: ${Math.floor(data.expires_in / 3600)} 小時
            `;
        }
        
        updateProfileDisplay(badge, data.is_manager);
        
    } catch (error) {
        alert('登入失敗: ' + error.message);
    }
}

function updateProfileDisplay(badge, isManager = false) {
    const profileName = document.getElementById('profile-name');
    const profileStatus = document.getElementById('profile-status');
    if (profileName) {
        profileName.textContent = badge;
    }
    if (profileStatus && badge !== '訪客') {
        profileStatus.innerHTML = isManager ? '<span style="color:var(--primary)">👔 主管</span>' : '<span style="color:var(--text-secondary)">👤 員工</span>';
    }
}

// ============ UTILITIES ============
function formatTime(timeString) {
    if (!timeString) return '-';
    try {
        const date = new Date(timeString);
        return date.toLocaleTimeString('zh-TW', { hour: '2-digit', minute: '2-digit' });
    } catch {
        return timeString;
    }
}

function getDateDaysAgo(days) {
    const date = new Date();
    date.setDate(date.getDate() - days);
    return date.toISOString().split('T')[0];
}

// Set default dates on load
window.addEventListener('load', () => {
    const today = new Date().toISOString().split('T')[0];
    const attendanceDateInput = document.getElementById('attendance-date');
    const managerDateInput = document.getElementById('manager-date');
    const trendStartInput = document.getElementById('trend-start');
    const trendEndInput = document.getElementById('trend-end');
    
    if (attendanceDateInput) attendanceDateInput.value = today;
    if (managerDateInput) managerDateInput.value = today;
    if (trendStartInput) trendStartInput.value = getDateDaysAgo(7);
    if (trendEndInput) trendEndInput.value = today;
});

// Periodically test connection every 30 seconds
setInterval(() => {
    fetch(`${getApiUrl()}/api/healthz`, { method: 'GET' })
        .then(r => updateServerStatus(r.ok))
        .catch(() => updateServerStatus(false));
}, 30000);
