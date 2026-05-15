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

    // Manager Tab
    document.getElementById('btn-fetch-manager')?.addEventListener('click', fetchManagerTeam);

    // Trend Tab
    document.getElementById('btn-fetch-trend')?.addEventListener('click', fetchTrend);

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
        'manager-tab': '主管視野報表',
        'trend-tab': '趨勢分析',
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

// ============ ATTENDANCE REPORT ============
async function fetchAttendance() {
    const date = document.getElementById('attendance-date')?.value;
    
    try {
        let url = `${getReportUrl()}/v1/reports/attendance?as=${state.currentBadge}`;
        if (date) {
            url += `&date=${date}`;
        }
        
        const response = await fetch(url);
        const data = await response.json();
        
        if (!response.ok) {
            throw new Error(data.error || '查詢失敗');
        }
        
        displayAttendanceReport(data);
        
    } catch (error) {
        displayAttendanceError(error.message);
    }
}

function displayAttendanceReport(reports) {
    const statsContainer = document.getElementById('attendance-stats');
    const tbody = document.getElementById('attendance-tbody');
    
    if (!reports || reports.length === 0) {
        statsContainer.innerHTML = '<p class="placeholder">無資料</p>';
        tbody.innerHTML = '<tr class="empty"><td colspan="9">無結果</td></tr>';
        return;
    }
    
    const uniqueEmployees = new Set(reports.map(r => r.employee_id)).size;
    const totalSwipes = reports.reduce((sum, r) => sum + (r.swipe_count || 0), 0);
    const avgStayHours = (reports.reduce((sum, r) => sum + (r.stay_hours || 0), 0) / reports.length).toFixed(1);
    
    statsContainer.innerHTML = `
        <div class="stat-item">
            <div class="stat-item-value">${reports.length}</div>
            <div class="stat-item-label">總紀錄</div>
        </div>
        <div class="stat-item">
            <div class="stat-item-value">${uniqueEmployees}</div>
            <div class="stat-item-label">員工數</div>
        </div>
        <div class="stat-item">
            <div class="stat-item-value">${totalSwipes}</div>
            <div class="stat-item-label">刷卡次</div>
        </div>
        <div class="stat-item">
            <div class="stat-item-value">${avgStayHours}</div>
            <div class="stat-item-label">平均停留</div>
        </div>
    `;
    
    tbody.innerHTML = reports.map(report => {
        const identity = getRoleBadge(report);
        return `
        <tr>
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
}

function displayAttendanceError(message) {
    const statsContainer = document.getElementById('attendance-stats');
    const tbody = document.getElementById('attendance-tbody');
    
    statsContainer.innerHTML = `<div style="color: var(--danger);">❌ ${message}</div>`;
    tbody.innerHTML = '<tr class="empty"><td colspan="9">查詢失敗</td></tr>';
}

async function exportAttendanceExcel() {
    const date = document.getElementById('attendance-date')?.value;
    
    try {
        let url = `${getReportUrl()}/v1/reports/attendance/export`;
        if (date) {
            url += `?date=${date}`;
        }
        
        const response = await fetch(url);
        if (!response.ok) {
            throw new Error('匯出失敗');
        }
        
        const blob = await response.blob();
        const downloadUrl = URL.createObjectURL(blob);
        const link = document.createElement('a');
        link.href = downloadUrl;
        link.download = `attendance-${date || new Date().toISOString().split('T')[0]}.xlsx`;
        link.click();
        URL.revokeObjectURL(downloadUrl);
        
    } catch (error) {
        alert('匯出失敗: ' + error.message);
    }
}

// ============ MANAGER TEAM ============
async function fetchManagerTeam() {
    const badge = document.getElementById('manager-badge')?.value?.trim();
    const date = document.getElementById('manager-date')?.value;
    
    if (!badge) {
        alert('請輸入主管證件 ID');
        return;
    }
    
    try {
        let url = `${getReportUrl()}/v1/reports/manager-team?as=${badge}`;
        if (date) {
            url += `&date=${date}`;
        }
        
        const response = await fetch(url);
        const data = await response.json();
        
        if (response.status === 403) {
            throw new Error(`${badge} 無主管權限`);
        }
        if (!response.ok) {
            throw new Error(data.error || '查詢失敗');
        }
        
        displayManagerTeam(data);
        
    } catch (error) {
        displayManagerError(error.message);
    }
}

function displayManagerTeam(data) {
    const scopeDisplay = document.getElementById('manager-scope');
    const tbody = document.getElementById('manager-tbody');
    
    if (!scopeDisplay || !tbody) return;
    
    scopeDisplay.textContent = data.manager_scope || '-';
    
    const reports = data.reports || [];
    if (reports.length === 0) {
        tbody.innerHTML = '<tr class="empty"><td colspan="9">無下屬出席紀錄</td></tr>';
        return;
    }
    
    tbody.innerHTML = reports.map(report => {
        const identity = getRoleBadge(report);
        return `
        <tr>
            <td>${report.employee_id}</td>
            <td>${report.name || '-'}</td>
            <td>${identity}</td>
            <td>${report.org_path || '-'}</td>
            <td>${report.work_date || '-'}</td>
            <td>${formatTime(report.first_in)}</td>
            <td>${formatTime(report.last_out)}</td>
            <td><strong>${report.swipe_count}</strong></td>
            <td>${report.stay_hours ? report.stay_hours.toFixed(1) + ' hr' : '-'}</td>
        </tr>
    `;
    }).join('');
}

function displayManagerError(message) {
    const scopeDisplay = document.getElementById('manager-scope');
    const tbody = document.getElementById('manager-tbody');
    
    if (scopeDisplay) scopeDisplay.textContent = '查詢失敗';
    if (tbody) tbody.innerHTML = `<tr><td colspan="9" style="color: var(--danger); text-align: center;">❌ ${message}</td></tr>`;
}

// ============ TREND ANALYSIS ============
async function fetchTrend() {
    const startDate = document.getElementById('trend-start')?.value;
    const endDate = document.getElementById('trend-end')?.value;
    const asBadge = document.getElementById('trend-as')?.value?.trim();
    
    if (!startDate || !endDate) {
        alert('請選擇開始和結束日期');
        return;
    }
    
    const period = document.getElementById('trend-period')?.value || 'day';
    
    try {
        let url = `${getReportUrl()}/v1/reports/trend?start_date=${startDate}&end_date=${endDate}&period=${period}`;
        if (asBadge) {
            url += `&as=${asBadge}`;
        }
        
        const response = await fetch(url);
        const data = await response.json();
        
        if (!response.ok) {
            throw new Error(data.error || '查詢失敗');
        }
        
        displayTrendChart(data);
        
    } catch (error) {
        alert('趨勢查詢失敗: ' + error.message);
    }
}

function displayTrendChart(data) {
    const scopeDisplay = document.getElementById('trend-scope-display');
    if (scopeDisplay) {
        scopeDisplay.textContent = data.scope || '全廠範圍 (Global)';
    }

    const trends = data.trends || [];
    if (trends.length === 0) {
        alert('無趨勢資料');
        return;
    }
    
    const selectedMetric = document.getElementById('trend-metric')?.value || 'avg_stay_hrs';
    const labels = trends.map(t => t.bucket);
    
    let dataset = {};
    if (selectedMetric === 'avg_stay_hrs') {
        dataset = {
            label: '平均停留時數 (hrs)',
            data: trends.map(t => t.avg_stay_hrs),
            borderColor: '#1e40af',
            backgroundColor: 'rgba(30, 64, 175, 0.1)',
            tension: 0.4,
            fill: true
        };
    } else if (selectedMetric === 'head_count') {
        dataset = {
            label: '出勤人頭數 (persons)',
            data: trends.map(t => t.head_count),
            borderColor: '#059669',
            backgroundColor: 'rgba(5, 150, 105, 0.1)',
            tension: 0.4,
            fill: true
        };
    } else if (selectedMetric === 'total_swipes') {
        dataset = {
            label: '總刷卡次數 (counts)',
            data: trends.map(t => t.total_swipes),
            borderColor: '#fbbf24',
            backgroundColor: 'rgba(251, 191, 36, 0.1)',
            tension: 0.4,
            fill: true
        };
    }
    
    const metricUnits = {
        'avg_stay_hrs': '時數 (hrs)',
        'head_count': '人數 (persons)',
        'total_swipes': '次數 (counts)'
    };
    const currentUnit = metricUnits[selectedMetric] || '';
    const datasets = [dataset];
    
    if (state.trendChart) {
        state.trendChart.destroy();
    }
    
    const ctx = document.getElementById('trend-chart');
    if (!ctx) return;
    
    state.trendChart = new Chart(ctx, {
        type: 'line',
        data: { labels, datasets },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            plugins: {
                legend: { labels: { color: '#f1f5f9' } }
            },
            scales: {
                y: {
                    beginAtZero: true,
                    title: {
                        display: true,
                        text: currentUnit,
                        color: '#94a3b8',
                        font: { size: 12 }
                    },
                    ticks: { color: '#f1f5f9' },
                    grid: { color: 'rgba(71, 85, 105, 0.2)' }
                },
                x: {
                    ticks: { color: '#f1f5f9' },
                    grid: { color: 'rgba(71, 85, 105, 0.2)' }
                }
            }
        }
    });
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
