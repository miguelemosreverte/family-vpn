// Family VPN Dashboard - Multi-Dashboard Application

// =============== STATE ===============

let peers = [];
let diskData = [];
let volumeData = [];
let featureFlags = {};
let currentDashboard = 'health';
let pingHistory = [];
let pingHourChart = null;
let ping24hChart = null;
let eventBus = null; // EventBus client for Layer 0 events
let clientVersions = {}; // VPN IP -> Git commit
let serverVersion = 'unknown'; // Server's Git commit

// =============== INITIALIZATION ===============

async function init() {
    console.log('🚀 Family VPN Dashboard starting...');

    // Load feature flags
    await loadFeatureFlags();

    // Load saved theme
    const savedTheme = localStorage.getItem('theme');
    if (savedTheme === 'dark') {
        document.body.classList.add('dark-mode');
    }

    // Setup event listeners
    setupEventListeners();
    setupNavigation();

    // Load data for current dashboard
    await loadDashboardData();

    // Start auto-refresh
    startAutoRefresh();

    // Start hot-reload watcher if enabled
    if (featureFlags.features?.autoUpdate?.enabled) {
        startHotReload();
    }

    // Add hidden update hotkey
    setupUpdateHotkey();

    // Connect to server via WebSocket
    connectWebSocket();

    console.log('✅ Dashboard initialized');
}

// =============== NAVIGATION ===============

function setupNavigation() {
    const navItems = document.querySelectorAll('.nav-item');

    navItems.forEach(item => {
        item.addEventListener('click', () => {
            const dashboard = item.getAttribute('data-dashboard');
            switchDashboard(dashboard);
        });
    });
}

function switchDashboard(dashboard) {
    // Update nav items
    document.querySelectorAll('.nav-item').forEach(item => {
        item.classList.remove('active');
        if (item.getAttribute('data-dashboard') === dashboard) {
            item.classList.add('active');
        }
    });

    // Update dashboards
    document.querySelectorAll('.dashboard').forEach(dash => {
        dash.classList.remove('active');
    });

    const targetDashboard = document.getElementById(`${dashboard}-dashboard`);
    if (targetDashboard) {
        targetDashboard.classList.add('active');
    }

    // Update title
    const titles = {
        'home': 'Home',
        'health': 'Health Monitoring',
        'volumes': 'Storage Management',
        'versions': 'Version Control'
    };
    document.getElementById('dashboard-title').textContent = titles[dashboard] || 'Dashboard';

    currentDashboard = dashboard;

    // Load data for this dashboard
    loadDashboardData();
}

// =============== EVENTBUS CONNECTION ===============

function connectWebSocket() {
    // Get VPN IP from server (via IPC)
    window.vpnAPI.getServerInfo().then(serverInfo => {
        const vpnIP = serverInfo.vpn_ip || '10.8.0.2'; // Default to first client IP
        const serverURL = `wss://95.217.238.72:443/ws?vpn_ip=${vpnIP}`;

        console.log(`🔌 Connecting to EventBus: ${serverURL}`);

        // Create EventBus client with subscriptions to all events
        const { NS, SUB, createEventBus } = window.EventBus;

        eventBus = createEventBus({
            url: serverURL,
            subscriptions: SUB.ALL, // Subscribe to all events
            autoReconnect: true,
            reconnectDelay: 1000,
            maxReconnectDelay: 30000,
            onConnect: () => {
                console.log('✅ EventBus connected');
                updateWebSocketStatus(true);
                // Server sends snapshot automatically on connect (per CLAUDE.md design)
                // No explicit request needed - just wait for onSnapshot callback
            },
            onDisconnect: () => {
                console.log('🔌 EventBus disconnected');
                updateWebSocketStatus(false);
            },
            onSnapshot: (snapshot) => {
                console.log('📸 Received snapshot:', snapshot.id);

                // Update state from snapshot
                if (snapshot.has('versions')) {
                    clientVersions = snapshot.get('versions') || {};
                    if (currentDashboard === 'versions') {
                        renderVersionsDashboardFromWebSocket();
                    }
                }
                if (snapshot.has('health')) {
                    const healthState = snapshot.get('health');
                    if (healthState && healthState.pingHistory) {
                        pingHistory = healthState.pingHistory;
                        if (currentDashboard === 'health') {
                            renderHealthCharts();
                            renderPingHistoryTable();
                            renderHealthMetrics();
                        }
                    }
                }
            },
            onError: (error) => {
                console.error('❌ EventBus error:', error);
                updateWebSocketStatus(false);
            }
        });

        // Subscribe to health events
        eventBus.subscribe('health.*', handleHealthEvent);

        // Subscribe to version events
        eventBus.subscribe('versions.*', handleVersionEvent);

        // Subscribe to update events
        eventBus.subscribe('updates.*', handleUpdateEvent);

        // Subscribe to system events
        eventBus.subscribe('system.*', handleSystemEvent);

        // Subscribe to peer events
        eventBus.subscribe('peers.*', handlePeerEvent);

        // Connect
        eventBus.connect();

    }).catch(error => {
        console.error('Failed to get server info:', error);
        // Retry connection in 5 seconds
        setTimeout(connectWebSocket, 5000);
    });
}

// =============== EVENT HANDLERS ===============

function handleHealthEvent(event) {
    const { NS } = window.EventBus;

    if (event.ns === NS.HEALTH_PING || event.ns === 'health.ping') {
        // Real-time ping update
        const newPing = {
            timestamp: event.ts,
            target: event.data.target,
            latency: event.data.latency,
            success: event.data.success !== false
        };

        pingHistory.push(newPing);

        // Keep only last 2880 records (24 hours)
        if (pingHistory.length > 2880) {
            pingHistory = pingHistory.slice(-2880);
        }

        // Update UI if on health dashboard
        if (currentDashboard === 'health') {
            renderHealthCharts();
            renderPingHistoryTable();
            renderHealthMetrics();
        }
    }
}

function handleVersionEvent(event) {
    const { NS } = window.EventBus;

    console.log(`📦 Version event: ${event.ns}`, event.data);

    if (event.data.vpn_ip && event.data.commit) {
        clientVersions[event.data.vpn_ip] = event.data.commit;
    }
    if (event.data.server_version) {
        serverVersion = event.data.server_version;
    }
    if (event.data.versions) {
        clientVersions = event.data.versions;
    }

    // Re-render versions dashboard if we're on it
    if (currentDashboard === 'versions') {
        renderVersionsDashboardFromWebSocket();
    }
}

function handleUpdateEvent(event) {
    console.log('🔄 Update event:', event.ns, event.data);

    // Handle Layer 0 Update Protocol message
    if (event.data) {
        handleUpdateMessage(event.data);
    }
}

function handleSystemEvent(event) {
    const { NS } = window.EventBus;

    console.log(`🔧 System event: ${event.ns}`, event.data);

    if (event.ns === 'system.connect' || event.ns === NS.SYSTEM_CONNECT) {
        // A client connected
        showNotification(`Client connected: ${event.data.vpn_ip || 'unknown'}`, 'info');
    } else if (event.ns === 'system.disconnect' || event.ns === NS.SYSTEM_DISCONNECT) {
        // A client disconnected
        showNotification(`Client disconnected: ${event.data.vpn_ip || 'unknown'}`, 'info');
    }
}

function handlePeerEvent(event) {
    console.log(`👥 Peer event: ${event.ns}`, event.data);

    // Refresh peer list
    loadDashboardData();
}

// Layer 0 Update Protocol - Domain definitions
const UpdateDomain = {
    ALL: 'all',
    CORE: 'core',
    SERVER: 'server',
    DESKTOP: 'desktop',
    UI: 'ui',
    MENUBAR: 'menubar',
    EXTENSION: 'extension'
};

const UpdateAction = {
    RELOAD: 'reload',
    RESTART: 'restart',
    NOTIFY: 'notify'
};

// Returns true if this receiver should handle the update
function shouldHandleUpdate(msg, receiverDomain, receiverTarget = '') {
    if (msg.domain === UpdateDomain.ALL) return true;
    if (msg.domain === receiverDomain) {
        if (msg.domain === UpdateDomain.EXTENSION && msg.target) {
            return msg.target === receiverTarget;
        }
        return true;
    }
    return false;
}

function handleWebSocketMessage(msg) {
    switch (msg.type) {
        case 'ping_history':
            // Received full ping history from server
            console.log(`📊 Received ${msg.history.length} ping records`);
            pingHistory = msg.history;

            // Re-render health dashboard if we're on it
            if (currentDashboard === 'health') {
                renderHealthCharts();
                renderPingHistoryTable();
                renderHealthMetrics();
            }
            break;

        case 'ping_update':
            // Real-time ping update
            const newPing = {
                timestamp: msg.timestamp,
                target: msg.target,
                latency: msg.latency,
                success: msg.success
            };

            pingHistory.push(newPing);

            // Keep only last 2880 records (24 hours)
            if (pingHistory.length > 2880) {
                pingHistory = pingHistory.slice(-2880);
            }

            // Update UI if on health dashboard
            if (currentDashboard === 'health') {
                renderHealthCharts();
                renderPingHistoryTable();
                renderHealthMetrics();
            }
            break;

        case 'update_available':
            console.log('🔄 Update available:', msg.version);
            // Auto-update will be handled by the client process
            break;

        case 'client_versions':
            // Response from server with all client versions
            console.log(`📦 Received client versions:`, msg.versions);
            clientVersions = msg.versions || {};
            if (msg.server_version) {
                serverVersion = msg.server_version;
            }
            // Re-render versions dashboard if we're on it
            if (currentDashboard === 'versions') {
                renderVersionsDashboardFromWebSocket();
            }
            break;

        case 'update':
            // Layer 0 Update Protocol message
            handleUpdateMessage(msg.payload);
            break;

        default:
            console.log('Unknown message type:', msg.type);
    }
}

// Handle Layer 0 Update Protocol messages
function handleUpdateMessage(updateMsg) {
    console.log(`🔄 Layer 0 Update: domain=${updateMsg.domain} action=${updateMsg.action} commit=${updateMsg.commit?.substring(0, 8)}`);

    // Check if this message is for us (UI domain)
    if (!shouldHandleUpdate(updateMsg, UpdateDomain.UI) &&
        !shouldHandleUpdate(updateMsg, UpdateDomain.DESKTOP)) {
        console.log(`   → Ignoring: not for UI/Desktop domain`);
        return;
    }

    // Log changed files
    if (updateMsg.files && updateMsg.files.length > 0) {
        console.log(`   → Changed files:`);
        updateMsg.files.forEach(f => console.log(`      - ${f}`));
    }

    // Determine action based on domain and action
    if (updateMsg.domain === UpdateDomain.UI ||
        (updateMsg.domain === UpdateDomain.ALL && updateMsg.action === UpdateAction.RELOAD)) {
        // UI updates: hot-reload without restart
        console.log('🎨 Hot-reloading UI...');
        handleUIHotReload(updateMsg);
    } else if (updateMsg.domain === UpdateDomain.DESKTOP ||
               updateMsg.action === UpdateAction.RESTART) {
        // Desktop updates: need app restart
        console.log('🔄 Desktop update requires restart');
        showNotification(`Update available: ${updateMsg.msg || 'New version'}. Restart to apply.`, 'info');
    }

    // Show notification for all updates
    if (updateMsg.msg) {
        console.log(`   → Message: ${updateMsg.msg}`);
    }
}

// Handle UI hot-reload (CSS and potentially JS)
async function handleUIHotReload(updateMsg) {
    // Check if only CSS files changed
    const cssOnly = updateMsg.files && updateMsg.files.every(f =>
        f.endsWith('.css') || f.includes('styles')
    );

    if (cssOnly) {
        // Hot-reload CSS only
        reloadCSS();
        showNotification('Styles updated!', 'success');
    } else {
        // For JS changes, we need to reload the page
        // But first, pull the latest from git
        try {
            const success = await window.vpnAPI.pullUIUpdates();
            if (success) {
                // Reload CSS first (instant)
                reloadCSS();

                // Then reload the page for JS changes
                showNotification('UI updated! Reloading...', 'success');
                setTimeout(() => {
                    location.reload();
                }, 1500);
            }
        } catch (error) {
            console.error('Failed to pull UI updates:', error);
            showNotification('Update failed. Try manual refresh.', 'error');
        }
    }
}

// =============== DATA LOADING ===============

async function loadDashboardData() {
    switch (currentDashboard) {
        case 'health':
            await loadHealthData();
            break;
        case 'volumes':
            await loadVolumesData();
            break;
        case 'versions':
            await loadVersionsData();
            break;
        case 'home':
            // No data to load for home
            break;
    }

    updateLastUpdated();
}

async function loadHealthData() {
    try {
        // Get VPN peers
        peers = await window.vpnAPI.getPeers();

        // Ping history will be loaded via WebSocket
        // (No need to generate mock data - it comes from server)

        // Render health dashboard
        renderHealthMetrics();
        renderHealthCharts();
        renderPeerStatus();
        renderPingHistoryTable();
    } catch (error) {
        console.error('Failed to load health data:', error);
    }
}

async function loadVolumesData() {
    try {
        // Get VPN peers
        peers = await window.vpnAPI.getPeers();
        console.log(`✅ Loaded ${peers.length} peers`);

        // Update header
        document.getElementById('client-count').textContent = peers.length;

        // Get disk usage for each peer (in parallel for speed)
        diskData = await Promise.all(
            peers.map(peer => window.vpnAPI.getDiskUsage(peer))
        );

        // Get volumes for each peer (in parallel for speed)
        volumeData = await Promise.all(
            peers.map(peer => window.vpnAPI.getVolumes(peer))
        );

        // Render volumes dashboard
        renderClients();
        renderVolumes();
        updateStats();
    } catch (error) {
        console.error('Failed to load volumes data:', error);
    }
}

async function loadVersionsData() {
    try {
        // Request client versions via EventBus
        if (eventBus && eventBus.connected) {
            console.log('📦 Requesting client versions from server...');
            eventBus.send('get_client_versions');
        }

        // Also get local Git commit for comparison
        const localInfo = await getVersionInfo();
        serverVersion = localInfo.server; // Use local git as server reference

        // Render with whatever data we have (will be updated when EventBus responds)
        renderVersionsDashboardFromWebSocket();
    } catch (error) {
        console.error('Failed to load versions data:', error);
    }
}

// =============== HEALTH DASHBOARD ===============

function renderHealthMetrics() {
    // Update client count
    document.getElementById('health-client-count').textContent = peers.length;

    // Mock server health (will be replaced with real data)
    const serverHealthElement = document.getElementById('server-health');
    const serverLatencyElement = document.getElementById('server-latency');

    const avgLatency = pingHistory.length > 0
        ? Math.round(pingHistory.reduce((sum, p) => sum + p.latency, 0) / pingHistory.length)
        : 0;

    serverHealthElement.innerHTML = `<span class="status-badge status-healthy">Healthy</span>`;
    serverLatencyElement.textContent = `Latency: ${avgLatency}ms`;

    // Calculate 24h uptime
    const uptime24h = calculateUptime(pingHistory.filter(p => p.timestamp > Date.now() - 24 * 60 * 60 * 1000));
    document.getElementById('uptime-24h').textContent = `${uptime24h.toFixed(1)}%`;

    // Calculate average latency for last hour
    const hourLatencies = pingHistory
        .filter(p => p.timestamp > Date.now() - 60 * 60 * 1000)
        .map(p => p.latency);

    const avgHourLatency = hourLatencies.length > 0
        ? Math.round(hourLatencies.reduce((a, b) => a + b, 0) / hourLatencies.length)
        : 0;

    document.getElementById('avg-latency').textContent = `${avgHourLatency} ms`;
}

function renderHealthCharts() {
    // Destroy existing charts
    if (pingHourChart) {
        pingHourChart.destroy();
    }
    if (ping24hChart) {
        ping24hChart.destroy();
    }

    // Last hour chart (1-minute intervals)
    const hourData = aggregatePingData(pingHistory, 60, 60); // 60 points, 1 min intervals
    pingHourChart = new Chart(document.getElementById('ping-hour-chart'), {
        type: 'line',
        data: {
            labels: hourData.labels,
            datasets: [{
                label: 'Latency (ms)',
                data: hourData.values,
                borderColor: '#0a84ff',
                backgroundColor: 'rgba(10, 132, 255, 0.1)',
                tension: 0.4,
                fill: true
            }]
        },
        options: getChartOptions('Last Hour')
    });

    // Last 24h chart (5-minute intervals)
    const dayData = aggregatePingData(pingHistory, 288, 300); // 288 points, 5 min intervals
    ping24hChart = new Chart(document.getElementById('ping-24h-chart'), {
        type: 'line',
        data: {
            labels: dayData.labels,
            datasets: [{
                label: 'Latency (ms)',
                data: dayData.values,
                borderColor: '#0a84ff',
                backgroundColor: 'rgba(10, 132, 255, 0.1)',
                tension: 0.4,
                fill: true
            }]
        },
        options: getChartOptions('Last 24 Hours')
    });
}

function renderPeerStatus() {
    const container = document.getElementById('peers-status-grid');

    if (peers.length === 0) {
        container.innerHTML = '<p style="grid-column: 1/-1; text-align: center; color: #86868b;">No peers connected</p>';
        return;
    }

    container.innerHTML = peers.map(peer => {
        const latency = Math.floor(Math.random() * 100) + 20; // Mock latency
        const statusClass = latency < 100 ? 'status-healthy' : latency < 200 ? 'status-degraded' : 'status-unhealthy';
        const statusText = latency < 100 ? 'Healthy' : latency < 200 ? 'Degraded' : 'Unhealthy';

        return `
            <div class="peer-status-card">
                <div class="peer-info">
                    <h4>${peer.hostname}</h4>
                    <p>${peer.vpn_address}</p>
                </div>
                <div class="peer-status">
                    <div class="status-badge ${statusClass}">${statusText}</div>
                    <p style="font-size: 12px; color: #86868b; margin-top: 4px;">${latency}ms</p>
                </div>
            </div>
        `;
    }).join('');
}

function renderPingHistoryTable() {
    const container = document.getElementById('ping-history-table');
    const recentPings = pingHistory.slice(-100).reverse();

    if (recentPings.length === 0) {
        container.innerHTML = '<p style="text-align: center; padding: 20px; color: #86868b;">No ping history available</p>';
        return;
    }

    const tableHTML = `
        <table>
            <thead>
                <tr>
                    <th>Timestamp</th>
                    <th>Target</th>
                    <th>Latency</th>
                    <th>Status</th>
                </tr>
            </thead>
            <tbody>
                ${recentPings.map(ping => {
                    const date = new Date(ping.timestamp);
                    const statusClass = ping.latency < 100 ? 'status-healthy' : ping.latency < 500 ? 'status-degraded' : 'status-unhealthy';
                    const statusText = ping.success ? 'Success' : 'Failed';

                    return `
                        <tr>
                            <td>${date.toLocaleTimeString()}</td>
                            <td>${ping.target}</td>
                            <td>${ping.latency}ms</td>
                            <td><span class="status-badge ${statusClass}">${statusText}</span></td>
                        </tr>
                    `;
                }).join('')}
            </tbody>
        </table>
    `;

    container.innerHTML = tableHTML;
}

// =============== VOLUMES DASHBOARD ===============

function renderClients() {
    const container = document.getElementById('clients-grid');

    if (peers.length === 0) {
        container.innerHTML = '<p style="grid-column: 1/-1; text-align: center; color: #86868b;">No peers connected</p>';
        return;
    }

    container.innerHTML = peers.map((peer, index) => {
        const disk = diskData[index];
        const hasError = disk && disk.error;

        let diskHTML = '';
        if (hasError) {
            diskHTML = `<div class="client-error">❌ ${disk.error}</div>`;
        } else if (disk && disk.usePercent) {
            const percent = parseInt(disk.usePercent);
            const fillClass = percent > 90 ? 'danger' : percent > 75 ? 'warning' : '';

            diskHTML = `
                <div class="disk-usage">
                    <h4>💾 Disk Usage</h4>
                    <div class="usage-bar">
                        <div class="usage-fill ${fillClass}" style="width: ${percent}%"></div>
                    </div>
                    <div class="usage-text">
                        ${disk.totalUsed} / ${disk.totalSize} (${disk.usePercent})
                    </div>
                </div>
            `;
        }

        return `
            <div class="client-card">
                <div class="client-header">
                    <div class="client-name">${peer.hostname}</div>
                    <div class="client-status">💚</div>
                </div>
                <div class="client-details">
                    <div class="client-detail">🌐 ${peer.vpn_address}</div>
                </div>
                ${diskHTML}
            </div>
        `;
    }).join('');
}

function renderVolumes() {
    const container = document.getElementById('volumes-container');

    const volumeCards = volumeData.map((volData, index) => {
        if (!volData || volData.error) {
            return `
                <div class="volume-card error-card">
                    <h3>❌ ${peers[index].hostname}</h3>
                    <p>${volData?.error || 'Failed to fetch volumes'}</p>
                </div>
            `;
        }

        const volumesHTML = volData.volumes.map(vol => `
            <tr>
                <td>${vol.mountPoint}</td>
                <td>${vol.size}</td>
                <td>${vol.used}</td>
                <td>${vol.available}</td>
                <td><strong>${vol.usePercent}</strong></td>
            </tr>
        `).join('');

        return `
            <div class="volume-card">
                <h3>💾 ${peers[index].hostname}</h3>
                <table>
                    <thead>
                        <tr>
                            <th>Mount Point</th>
                            <th>Size</th>
                            <th>Used</th>
                            <th>Available</th>
                            <th>Use %</th>
                        </tr>
                    </thead>
                    <tbody>
                        ${volumesHTML}
                    </tbody>
                </table>
            </div>
        `;
    }).join('');

    container.innerHTML = volumeCards;
}

function updateStats() {
    let totalSizeGB = 0;
    diskData.forEach(disk => {
        if (disk && !disk.error) {
            totalSizeGB += parseSize(disk.totalSize);
        }
    });

    document.getElementById('total-disk').textContent = `${Math.round(totalSizeGB)} GB`;
}

function parseSize(sizeStr) {
    if (!sizeStr) return 0;
    const num = parseFloat(sizeStr);
    if (sizeStr.includes('Ti') || sizeStr.includes('T')) {
        return num * 1024;
    } else if (sizeStr.includes('Gi') || sizeStr.includes('G')) {
        return num;
    } else if (sizeStr.includes('Mi') || sizeStr.includes('M')) {
        return num / 1024;
    }
    return num;
}

// =============== VERSIONS DASHBOARD ===============

async function getVersionInfo() {
    // Mock version data for now
    const exec = require('child_process').exec;

    return new Promise((resolve) => {
        exec('git rev-parse HEAD', (err, stdout) => {
            const currentCommit = err ? 'unknown' : stdout.trim().substring(0, 8);

            resolve({
                server: currentCommit,
                clients: peers.map(peer => ({
                    hostname: peer.hostname,
                    version: currentCommit, // In production, this would come from WebSocket
                    status: 'up-to-date',
                    lastUpdate: new Date()
                }))
            });
        });
    });
}

function renderVersionsDashboard(versionInfo) {
    // Update server version
    document.getElementById('server-version').innerHTML = `<code>${versionInfo.server}</code>`;

    // Update deployment stats
    const upToDate = versionInfo.clients.filter(c => c.status === 'up-to-date').length;
    const updating = versionInfo.clients.filter(c => c.status === 'updating').length;
    const behind = versionInfo.clients.filter(c => c.status === 'behind').length;

    document.getElementById('up-to-date-count').textContent = upToDate;
    document.getElementById('updating-count').textContent = updating;
    document.getElementById('behind-count').textContent = behind;

    // Render client versions table
    const tableHTML = `
        <table>
            <thead>
                <tr>
                    <th>Client</th>
                    <th>Version</th>
                    <th>Status</th>
                    <th>Last Update</th>
                </tr>
            </thead>
            <tbody>
                ${versionInfo.clients.map(client => `
                    <tr>
                        <td>${client.hostname}</td>
                        <td><code>${client.version}</code></td>
                        <td><span class="status-badge status-healthy">${client.status}</span></td>
                        <td>${client.lastUpdate.toLocaleString()}</td>
                    </tr>
                `).join('')}
            </tbody>
        </table>
    `;

    document.getElementById('clients-version-table').innerHTML = tableHTML;
}

// Render versions dashboard using WebSocket data
function renderVersionsDashboardFromWebSocket() {
    // Update server version
    const serverVersionEl = document.getElementById('server-version');
    if (serverVersionEl) {
        const shortVersion = serverVersion.length > 8 ? serverVersion.substring(0, 8) : serverVersion;
        serverVersionEl.innerHTML = `<code>${shortVersion}</code>`;
    }

    // Build clients list from peers + clientVersions
    const clientsList = [];
    const versionCounts = { upToDate: 0, updating: 0, behind: 0 };

    // Use peers list as source of connected clients
    for (const peer of peers) {
        const vpnIP = peer.vpn_address;
        const clientCommit = clientVersions[vpnIP] || 'unknown';
        const shortCommit = clientCommit.length > 8 ? clientCommit.substring(0, 8) : clientCommit;

        // Determine status by comparing to server version
        let status = 'unknown';
        let statusClass = 'status-unknown';

        if (clientCommit === 'unknown') {
            status = 'Unknown';
            statusClass = 'status-unknown';
        } else if (clientCommit === serverVersion) {
            status = 'Up to date';
            statusClass = 'status-healthy';
            versionCounts.upToDate++;
        } else {
            status = 'Behind';
            statusClass = 'status-degraded';
            versionCounts.behind++;
        }

        clientsList.push({
            hostname: peer.hostname,
            vpnIP: vpnIP,
            version: shortCommit,
            fullVersion: clientCommit,
            status: status,
            statusClass: statusClass,
            os: peer.os || 'unknown'
        });
    }

    // Also add any clients from clientVersions that aren't in peers
    for (const [vpnIP, commit] of Object.entries(clientVersions)) {
        const alreadyAdded = clientsList.some(c => c.vpnIP === vpnIP);
        if (!alreadyAdded) {
            const shortCommit = commit.length > 8 ? commit.substring(0, 8) : commit;
            const status = commit === serverVersion ? 'Up to date' : 'Behind';
            const statusClass = commit === serverVersion ? 'status-healthy' : 'status-degraded';

            if (commit === serverVersion) {
                versionCounts.upToDate++;
            } else {
                versionCounts.behind++;
            }

            clientsList.push({
                hostname: vpnIP, // Use IP as hostname if not in peers
                vpnIP: vpnIP,
                version: shortCommit,
                fullVersion: commit,
                status: status,
                statusClass: statusClass,
                os: 'unknown'
            });
        }
    }

    // Update deployment stats
    document.getElementById('up-to-date-count').textContent = versionCounts.upToDate;
    document.getElementById('updating-count').textContent = versionCounts.updating;
    document.getElementById('behind-count').textContent = versionCounts.behind;

    // Render client versions table
    const container = document.getElementById('clients-version-table');
    if (!container) return;

    if (clientsList.length === 0) {
        container.innerHTML = `
            <p style="text-align: center; padding: 40px; color: #86868b;">
                No clients connected yet. Client versions will appear here when they connect via VPN.
            </p>
        `;
        return;
    }

    const tableHTML = `
        <table>
            <thead>
                <tr>
                    <th>Client</th>
                    <th>VPN IP</th>
                    <th>OS</th>
                    <th>Version</th>
                    <th>Status</th>
                </tr>
            </thead>
            <tbody>
                ${clientsList.map(client => `
                    <tr>
                        <td>${client.hostname}</td>
                        <td><code>${client.vpnIP}</code></td>
                        <td>${getOSEmoji(client.os)} ${client.os}</td>
                        <td><code title="${client.fullVersion}">${client.version}</code></td>
                        <td><span class="status-badge ${client.statusClass}">${client.status}</span></td>
                    </tr>
                `).join('')}
            </tbody>
        </table>
    `;

    container.innerHTML = tableHTML;
}

// Helper to get OS emoji
function getOSEmoji(os) {
    switch (os.toLowerCase()) {
        case 'darwin': return '🍎';
        case 'linux': return '🐧';
        case 'windows': return '🪟';
        default: return '💻';
    }
}

// =============== MOCK DATA GENERATORS ===============

function generateMockPingHistory() {
    const history = [];
    const now = Date.now();

    // Generate last 24 hours of ping data (every 30 seconds)
    for (let i = 0; i < 2880; i++) {
        history.push({
            timestamp: now - i * 30 * 1000,
            target: '95.217.238.72',
            latency: Math.floor(Math.random() * 80) + 20, // 20-100ms
            success: Math.random() > 0.02 // 98% success rate
        });
    }

    return history.reverse();
}

function aggregatePingData(pings, points, intervalSeconds) {
    const now = Date.now();
    const labels = [];
    const values = [];

    for (let i = points - 1; i >= 0; i--) {
        const endTime = now - (i * intervalSeconds * 1000);
        const startTime = endTime - (intervalSeconds * 1000);

        const periodPings = pings.filter(p =>
            p.timestamp >= startTime && p.timestamp < endTime && p.success
        );

        const avgLatency = periodPings.length > 0
            ? periodPings.reduce((sum, p) => sum + p.latency, 0) / periodPings.length
            : null;

        const date = new Date(endTime);
        if (intervalSeconds < 120) {
            labels.push(date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }));
        } else {
            labels.push(date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }));
        }

        values.push(avgLatency);
    }

    return { labels, values };
}

function calculateUptime(pings) {
    if (pings.length === 0) return 100;
    const successful = pings.filter(p => p.success).length;
    return (successful / pings.length) * 100;
}

function getChartOptions(title) {
    const isDark = document.body.classList.contains('dark-mode');

    return {
        responsive: true,
        maintainAspectRatio: true,
        plugins: {
            legend: {
                display: false
            },
            tooltip: {
                mode: 'index',
                intersect: false
            }
        },
        scales: {
            x: {
                grid: {
                    color: isDark ? '#2c2c2e' : '#e5e5e7'
                },
                ticks: {
                    color: isDark ? '#86868b' : '#1d1d1f',
                    maxRotation: 0
                }
            },
            y: {
                grid: {
                    color: isDark ? '#2c2c2e' : '#e5e5e7'
                },
                ticks: {
                    color: isDark ? '#86868b' : '#1d1d1f'
                },
                beginAtZero: true
            }
        }
    };
}

// =============== EVENT LISTENERS ===============

function setupEventListeners() {
    // Theme toggle
    document.getElementById('theme-toggle').addEventListener('click', () => {
        document.body.classList.toggle('dark-mode');
        const theme = document.body.classList.contains('dark-mode') ? 'dark' : 'light';
        localStorage.setItem('theme', theme);

        // Recreate charts with new colors
        if (currentDashboard === 'health') {
            renderHealthCharts();
        }
    });

    // Refresh button
    document.getElementById('refresh-btn').addEventListener('click', () => {
        loadDashboardData();
    });
}

function updateLastUpdated() {
    const now = new Date();
    document.getElementById('last-updated').textContent = now.toLocaleTimeString();
}

function updateWebSocketStatus(connected) {
    const indicator = document.getElementById('ws-status');
    const statusText = document.querySelector('.connection-status .status-text');
    const wsConnection = document.getElementById('ws-connection');

    if (connected) {
        indicator.classList.add('connected');
        statusText.textContent = 'Connected';
        wsConnection.textContent = 'Connected';
    } else {
        indicator.classList.remove('connected');
        statusText.textContent = 'Disconnected';
        wsConnection.textContent = 'Disconnected';
    }
}

// =============== AUTO-REFRESH ===============

function startAutoRefresh() {
    setInterval(() => {
        loadDashboardData();
    }, 30000); // Refresh every 30 seconds
}

// =============== FEATURE FLAGS ===============

async function loadFeatureFlags() {
    try {
        featureFlags = await window.vpnAPI.getFeatureFlags();
        console.log('✅ Feature flags loaded:', featureFlags);
    } catch (error) {
        console.warn('⚠️  Failed to load feature flags, using defaults:', error);
        featureFlags = {
            version: '1.0.0',
            features: {
                autoUpdate: { enabled: true, checkIntervalMinutes: 5 }
            }
        };
    }
}

// =============== HOT RELOAD ===============

function startHotReload() {
    const checkInterval = (featureFlags.features?.autoUpdate?.checkIntervalMinutes || 5) * 60 * 1000;

    console.log(`🔥 Hot-reload enabled (checking every ${checkInterval / 60000} minutes)`);

    setInterval(async () => {
        try {
            const hasUpdates = await window.vpnAPI.checkForUIUpdates();

            if (hasUpdates) {
                console.log('🔄 UI updates detected, pulling from Git...');
                const success = await window.vpnAPI.pullUIUpdates();

                if (success) {
                    console.log('✅ UI updated from Git, reloading...');
                    await loadFeatureFlags();
                    reloadCSS();
                    showNotification('UI updated from Git!', 'success');
                }
            }
        } catch (error) {
            console.error('❌ Hot-reload check failed:', error);
        }
    }, checkInterval);
}

function reloadCSS() {
    const links = document.querySelectorAll('link[rel="stylesheet"]');
    links.forEach(link => {
        const href = link.getAttribute('href').split('?')[0];
        link.setAttribute('href', href + '?reload=' + new Date().getTime());
    });
    console.log('🎨 CSS reloaded');
}

// =============== UPDATE HOTKEY ===============

function setupUpdateHotkey() {
    document.addEventListener('keydown', async (e) => {
        if ((e.metaKey || e.ctrlKey) && e.shiftKey && e.key === 'U') {
            e.preventDefault();
            console.log('🔧 Update/Reinstall triggered by hotkey');

            const confirmed = confirm('Update/Reinstall Family VPN?\n\nThis will:\n1. Pull latest changes from Git\n2. Rebuild and reinstall both apps\n3. Restart the application\n\nContinue?');

            if (confirmed) {
                showNotification('Starting update/reinstall...', 'info');

                try {
                    await window.vpnAPI.triggerReinstall();
                } catch (error) {
                    console.error('❌ Reinstall failed:', error);
                    showNotification('Update failed: ' + error.message, 'error');
                }
            }
        }
    });

    console.log('⌨️  Hidden update hotkey registered (Cmd+Shift+U)');
}

// =============== NOTIFICATIONS ===============

function showNotification(message, type = 'info') {
    const banner = document.createElement('div');
    banner.className = `notification notification-${type}`;
    banner.innerHTML = `
        <span>${message}</span>
        <button onclick="this.parentElement.remove()">✕</button>
    `;
    banner.style.cssText = `
        position: fixed;
        top: 20px;
        right: 20px;
        padding: 15px 20px;
        background: ${type === 'success' ? '#4caf50' : type === 'error' ? '#f44336' : '#2196f3'};
        color: white;
        border-radius: 8px;
        box-shadow: 0 4px 12px rgba(0,0,0,0.3);
        z-index: 10000;
        animation: slideIn 0.3s ease-out;
        display: flex;
        align-items: center;
        gap: 15px;
    `;

    document.body.appendChild(banner);

    setTimeout(() => {
        banner.style.opacity = '0';
        banner.style.transition = 'opacity 0.3s';
        setTimeout(() => banner.remove(), 300);
    }, 5000);
}

// =============== START APPLICATION ===============

document.addEventListener('DOMContentLoaded', init);
