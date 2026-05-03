// ============================================
// PACS Frontend - Application Logic
// ============================================

// ============ STATE MANAGEMENT ============
const state = {
    swipeHistory: [],
    apiUrl: localStorage.getItem('apiUrl') || '',
    reportUrl: localStorage.getItem('reportUrl') || '',
    lastDirection: 'IN'
};

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
    document.querySelectorAll('.tab-btn').forEach(btn => {
        btn.addEventListener('click', switchTab);
    });

    document.getElementById('btn-in').addEventListener('click', (e) => selectDirection(e, 'IN'));
    document.getElementById('btn-out').addEventListener('click', (e) => selectDirection(e, 'OUT'));
    document.getElementById('btn-swipe').addEventListener('click', sendSwipe);
    document.getElementById('btn-clear-history').addEventListener('click', clearHistory);
    document.getElementById('btn-export-history').addEventListener('click', exportHistory);
    document.getElementById('btn-fetch-report').addEventListener('click', fetchReport);
    document.getElementById('btn-test-connection').addEventListener('click', testServerConnection);
    document.getElementById('api-url').addEventListener('change', saveSettings);
    document.getElementById('report-url').addEventListener('change', saveSettings);
}

// ============ TAB SWITCHING ============
function switchTab(e) {
    const tabName = e.target.getAttribute('data-tab');
    document.querySelectorAll('.tab-btn').forEach(btn => btn.classList.remove('active'));
    document.querySelectorAll('.tab-content').forEach(tab => tab.classList.remove('active'));
    e.target.classList.add('active');
    document.getElementById(tabName).classList.add('active');
}

// ============ DIRECTION SELECTION ============
function selectDirection(e, direction) {
    document.getElementById('direction').value = direction;
    state.lastDirection = direction;
    document.querySelectorAll('.direction-btn').forEach(btn => btn.classList.remove('active'));
    e.target.classList.add('active');
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
    const badgeId = document.getElementById('badge-id').value.trim();
    const siteId = document.getElementById('site-id').value;
    const gateId = document.getElementById('gate-id').value;
    const direction = document.getElementById('direction').value;

    if (!badgeId) {
        alert('請輸入員工證件 ID');
        return;
    }

    const payload = {
        badge_id: badgeId,
        site_id: siteId,
        gate_id: gateId,
        direction: direction
    };

    try {
        const response = await fetch(`${getApiUrl()}/v1/swipe`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload)
        });

        const data = await response.json();
        displaySwipeResponse(response.status, data);
        addToSwipeHistory(badgeId, siteId, gateId, direction, data.status);

        setTimeout(() => {
            document.getElementById('direction').value = state.lastDirection;
            document.querySelectorAll('.direction-btn').forEach(btn => btn.classList.remove('active'));
            if (state.lastDirection === 'IN') {
                document.getElementById('btn-in').classList.add('active');
            } else {
                document.getElementById('btn-out').classList.add('active');
            }
        }, 500);

    } catch (error) {
        displaySwipeResponse(0, { error: error.message });
    }
}

// ============ SWIPE RESPONSE DISPLAY ============
function displaySwipeResponse(status, data) {
    const responseBox = document.getElementById('swipe-response');
    const responseContent = document.getElementById('response-content');
    const isSuccess = status === 200;

    responseBox.classList.remove('hidden', 'success', 'error');
    responseBox.classList.add(isSuccess ? 'success' : 'error');

    responseContent.innerHTML = `
        <strong>狀態碼:</strong> ${status}<br>
        <strong>結果:</strong> ${JSON.stringify(data, null, 2)}
    `;

    responseBox.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
}

// ============ SWIPE HISTORY ============
function addToSwipeHistory(badgeId, siteId, gateId, direction, status) {
    const record = {
        timestamp: new Date().toLocaleTimeString('zh-TW'),
        badgeId, siteId, gateId, direction, status
    };

    state.swipeHistory.unshift(record);
    if (state.swipeHistory.length > 50) state.swipeHistory.pop();

    saveSwipeHistory();
    renderSwipeHistory();
}

function saveSwipeHistory() {
    localStorage.setItem('swipeHistory', JSON.stringify(state.swipeHistory));
}

function loadSwipeHistory() {
    const saved = localStorage.getItem('swipeHistory');
    if (saved) {
        state.swipeHistory = JSON.parse(saved);
        renderSwipeHistory();
    }
}

function renderSwipeHistory() {
    const tbody = document.getElementById('history-tbody');

    if (state.swipeHistory.length === 0) {
        tbody.innerHTML = '<tr class="empty"><td colspan="5">還沒有刷卡紀錄</td></tr>';
        return;
    }

    tbody.innerHTML = state.swipeHistory.map(record => {
        const statusClass = record.status === 'SUCCESS' ? 'status-success' : 'status-error';
        const statusText = record.status === 'SUCCESS' ? '✓ 允許' : '✗ 拒絕';

        return `
            <tr>
                <td>${record.timestamp}</td>
                <td>${record.badgeId}</td>
                <td>${record.siteId}</td>
                <td>${record.direction === 'IN' ? '➡️ 進入' : '⬅️ 離開'}</td>
                <td class="${statusClass}">${statusText}</td>
            </tr>
        `;
    }).join('');
}

function clearHistory() {
    if (confirm('確定要清除所有刷卡紀錄嗎？')) {
        state.swipeHistory = [];
        saveSwipeHistory();
        renderSwipeHistory();
        alert('已清除刷卡紀錄');
    }
}

function exportHistory() {
    if (state.swipeHistory.length === 0) {
        alert('沒有可匯出的紀錄');
        return;
    }

    const headers = ['時間', '證件ID', '地點', '閘門', '方向', '狀態'];
    const rows = state.swipeHistory.map(r => [
        r.timestamp, r.badgeId, r.siteId, r.gateId, r.direction, r.status
    ]);

    let csv = headers.join(',') + '\n';
    rows.forEach(row => {
        csv += row.map(cell => `"${cell}"`).join(',') + '\n';
    });

    const blob = new Blob([csv], { type: 'text/csv;charset=utf-8;' });
    const link = document.createElement('a');
    link.href = URL.createObjectURL(blob);
    link.download = `swipe-history-${new Date().toISOString().split('T')[0]}.csv`;
    link.click();
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

// ============ UTILITIES ============
function formatTime(timeString) {
    if (!timeString) return '-';
    try {
        const date = new Date(timeString);
        return date.toLocaleTimeString('zh-TW');
    } catch {
        return timeString;
    }
}

// Periodically test connection every 30 seconds
setInterval(() => {
    fetch(`${getApiUrl()}/api/healthz`, { method: 'GET' })
        .then(r => updateServerStatus(r.ok))
        .catch(() => updateServerStatus(false));
}, 30000);
