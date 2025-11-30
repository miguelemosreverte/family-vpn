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
let latencyTimeSeriesChart = null;
let throughputTimeSeriesChart = null;
let eventBus = null; // EventBus client for Layer 0 events
let clientVersions = {}; // VPN IP -> Git commit
let serverVersion = 'unknown'; // Server's Git commit
let hostnameToIP = {}; // Hostname -> VPN IP mapping
let clientStats = {}; // Client statistics from daemon
let eventLogs = []; // Event logs buffer
let daemonHealth = null; // Daemon health status
let benchmarkData = null; // Benchmark results

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
    if (event.data.hostname_to_ip) {
        hostnameToIP = event.data.hostname_to_ip;
        populateClientSelects();
    }

    // Re-render versions dashboard if we're on it
    if (currentDashboard === 'versions') {
        renderVersionsDashboardFromWebSocket();
        renderHostnameMapping();
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
            if (msg.hostname_to_ip) {
                hostnameToIP = msg.hostname_to_ip;
                populateClientSelects();
            }
            // Re-render versions dashboard if we're on it
            if (currentDashboard === 'versions') {
                renderVersionsDashboardFromWebSocket();
                renderHostnameMapping();
            }
            break;

        case 'health':
            // Health data from daemon
            console.log('💚 Received health data:', msg);
            daemonHealth = msg;
            if (currentDashboard === 'health') {
                renderDaemonStatus();
            }
            break;

        case 'stats':
            // Client statistics from daemon
            console.log('📈 Received client stats:', msg);
            if (msg.clients) {
                clientStats = msg.clients;
            } else if (msg.client) {
                // Single client stats
                const hostname = msg.client.hostname;
                if (hostname) {
                    clientStats[hostname] = msg.client;
                }
            }
            if (currentDashboard === 'health') {
                renderClientStats();
            }
            break;

        case 'logs':
            // Event logs from daemon
            console.log(`📝 Received ${msg.events?.length || 0} log entries`);
            eventLogs = msg.events || [];
            if (currentDashboard === 'health') {
                renderEventLogs();
            }
            break;

        case 'benchmark':
            // Benchmark results
            console.log('⚡ Received benchmark data:', msg);
            benchmarkData = msg;
            if (currentDashboard === 'health') {
                renderBenchmarkData();
            }
            break;

        case 'timeseries':
            // Time series data
            console.log(`📈 Received time series: ${msg.series}`, msg);
            if (msg.series === 'latency') {
                renderLatencyTimeSeries(msg);
            } else if (msg.series === 'throughput') {
                renderThroughputTimeSeries(msg);
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

        // Populate client selects with peer data
        populateClientSelects();

        // Ping history will be loaded via WebSocket
        // (No need to generate mock data - it comes from server)

        // Render health dashboard
        renderHealthMetrics();
        renderHealthCharts();
        renderPeerStatus();
        renderPingHistoryTable();
        renderDaemonStatus();
        renderClientStats();
        renderBenchmarkData();
        renderEventLogs();

        // Load additional data from server
        loadDaemonHealth();
        loadClientStats();
        loadBenchmarkData();
        loadEventLogs();
        loadLatencyTimeSeries();
        loadThroughputTimeSeries();
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

        // Populate client selects
        populateClientSelects();

        // Render with whatever data we have (will be updated when EventBus responds)
        renderVersionsDashboardFromWebSocket();
        renderHostnameMapping();
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
                    <th>Actions</th>
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
                        <td class="action-cell">
                            <button class="action-btn-mini" onclick="updateClient('${client.vpnIP}')" title="Update to latest">Update</button>
                        </td>
                    </tr>
                `).join('')}
            </tbody>
        </table>
    `;

    container.innerHTML = tableHTML;
}

// Update a single client
async function updateClient(vpnIP) {
    try {
        if (eventBus && eventBus.connected) {
            eventBus.send('broadcast', {
                ns: 'updates.available',
                data: {
                    targets: [vpnIP],
                    version: '',
                    domain: 'all'
                }
            });
            showNotification(`Update triggered for ${vpnIP}`, 'success');
        }
    } catch (error) {
        console.error('Failed to update client:', error);
        showNotification('Failed to update client', 'error');
    }
}
window.updateClient = updateClient;

// Helper to get OS emoji
function getOSEmoji(os) {
    switch (os.toLowerCase()) {
        case 'darwin': return '🍎';
        case 'linux': return '🐧';
        case 'windows': return '🪟';
        default: return '💻';
    }
}

// =============== UPDATE ALL CLIENTS ===============

// Trigger update for all clients via server API
async function triggerUpdateAll() {
    const statusEl = document.getElementById('update-status');
    const button = document.getElementById('update-all-btn');

    // Disable button and show loading state
    button.disabled = true;
    button.textContent = 'Updating...';
    statusEl.textContent = 'Sending update command to all clients...';
    statusEl.style.color = '#0a84ff';

    try {
        // Call server's update API
        const response = await fetch('https://95.217.238.72:443/update/init?component=all', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            }
        });

        if (response.ok) {
            const result = await response.json();
            statusEl.textContent = `Update triggered! ${result.message || 'All clients notified.'}`;
            statusEl.style.color = '#30d158';
            showNotification('Update command sent to all clients', 'success');
        } else {
            throw new Error(`Server returned ${response.status}`);
        }
    } catch (error) {
        console.error('Failed to trigger update:', error);
        statusEl.textContent = `Error: ${error.message}`;
        statusEl.style.color = '#ff453a';
        showNotification('Failed to trigger update: ' + error.message, 'error');
    } finally {
        // Re-enable button after 3 seconds
        setTimeout(() => {
            button.disabled = false;
            button.textContent = 'Update All Clients';
        }, 3000);
    }
}

// Make it globally accessible for onclick handler
window.triggerUpdateAll = triggerUpdateAll;

// =============== TARGETED UPDATE ===============

async function triggerTargetedUpdate() {
    const targetSelect = document.getElementById('update-target-select');
    const versionInput = document.getElementById('update-version-input');
    const target = targetSelect.value;
    const version = versionInput.value.trim();

    if (!target) {
        showNotification('Please select a target client', 'error');
        return;
    }

    const button = document.getElementById('targeted-update-btn');
    button.disabled = true;
    button.textContent = 'Updating...';

    try {
        // Send update event via EventBus
        if (eventBus && eventBus.connected) {
            eventBus.send('broadcast', {
                ns: 'updates.available',
                data: {
                    targets: [target],
                    version: version || '',
                    domain: 'all'
                }
            });
            showNotification(`Update triggered for ${target}`, 'success');
        } else {
            throw new Error('EventBus not connected');
        }
    } catch (error) {
        console.error('Failed to trigger targeted update:', error);
        showNotification('Failed to trigger update: ' + error.message, 'error');
    } finally {
        setTimeout(() => {
            button.disabled = false;
            button.textContent = 'Update Selected Client';
        }, 2000);
    }
}
window.triggerTargetedUpdate = triggerTargetedUpdate;

// =============== ROLLBACK ===============

async function triggerRollback() {
    const targetSelect = document.getElementById('rollback-target-select');
    const commitInput = document.getElementById('rollback-commit-input');
    const statusEl = document.getElementById('rollback-status');
    const target = targetSelect.value;
    const commit = commitInput.value.trim();

    if (!target) {
        statusEl.textContent = 'Please select a target client';
        statusEl.className = 'action-status error';
        return;
    }

    if (!commit) {
        statusEl.textContent = 'Please enter a commit hash';
        statusEl.className = 'action-status error';
        return;
    }

    const button = document.getElementById('rollback-btn');
    button.disabled = true;
    button.textContent = 'Rolling back...';
    statusEl.textContent = 'Sending rollback command...';
    statusEl.className = 'action-status';

    try {
        // Send rollback event via EventBus
        if (eventBus && eventBus.connected) {
            eventBus.send('broadcast', {
                ns: 'updates.available',
                data: {
                    targets: [target],
                    version: commit,
                    rollback: true,
                    domain: 'all'
                }
            });
            statusEl.textContent = `Rollback triggered for ${target} to ${commit.substring(0, 8)}`;
            statusEl.className = 'action-status success';
            showNotification('Rollback command sent', 'success');
        } else {
            throw new Error('EventBus not connected');
        }
    } catch (error) {
        console.error('Failed to trigger rollback:', error);
        statusEl.textContent = 'Error: ' + error.message;
        statusEl.className = 'action-status error';
        showNotification('Failed to trigger rollback: ' + error.message, 'error');
    } finally {
        setTimeout(() => {
            button.disabled = false;
            button.textContent = 'Rollback Client';
        }, 2000);
    }
}
window.triggerRollback = triggerRollback;

// =============== SYNC VERSIONS ===============

async function syncVersions() {
    const button = document.getElementById('sync-versions-btn');
    const statusEl = document.getElementById('update-status');

    button.disabled = true;
    button.textContent = 'Syncing...';
    statusEl.textContent = 'Requesting version sync from all clients...';
    statusEl.className = 'action-status';

    try {
        if (eventBus && eventBus.connected) {
            eventBus.send('broadcast', {
                ns: 'versions.sync',
                data: {}
            });
            statusEl.textContent = 'Version sync requested';
            statusEl.className = 'action-status success';
            showNotification('Version sync requested', 'success');
        } else {
            throw new Error('EventBus not connected');
        }
    } catch (error) {
        console.error('Failed to sync versions:', error);
        statusEl.textContent = 'Error: ' + error.message;
        statusEl.className = 'action-status error';
    } finally {
        setTimeout(() => {
            button.disabled = false;
            button.textContent = 'Sync Versions';
        }, 2000);
    }
}
window.syncVersions = syncVersions;

// =============== REFRESH VERSIONS ===============

async function refreshVersions() {
    if (eventBus && eventBus.connected) {
        console.log('Requesting client versions...');
        eventBus.send('get_client_versions');
        showNotification('Refreshing versions...', 'info');
    }
}
window.refreshVersions = refreshVersions;

// =============== DAEMON HEALTH ===============

async function loadDaemonHealth() {
    try {
        // Request health data via EventBus
        if (eventBus && eventBus.connected) {
            eventBus.send('get_health');
        }
    } catch (error) {
        console.error('Failed to load daemon health:', error);
    }
}

function renderDaemonStatus() {
    const statusEl = document.getElementById('daemon-status');
    const wsStatusEl = document.getElementById('daemon-ws-status');
    const uptimeEl = document.getElementById('daemon-uptime');
    const reconnectsEl = document.getElementById('daemon-reconnects');
    const eventsEl = document.getElementById('daemon-events');

    if (!daemonHealth) {
        statusEl.textContent = 'Unknown';
        wsStatusEl.textContent = '--';
        uptimeEl.textContent = '--';
        reconnectsEl.textContent = '--';
        eventsEl.textContent = '--';
        return;
    }

    const daemon = daemonHealth.daemon || {};

    statusEl.textContent = daemon.connected ? 'CONNECTED' : 'DISCONNECTED';
    statusEl.className = `daemon-value ${daemon.connected ? 'connected' : 'disconnected'}`;

    wsStatusEl.textContent = daemon.connected ? 'Active' : 'Inactive';
    wsStatusEl.className = `daemon-value ${daemon.connected ? 'connected' : 'disconnected'}`;

    uptimeEl.textContent = daemon.uptime || '--';
    reconnectsEl.textContent = daemon.reconnects !== undefined ? daemon.reconnects : '--';
    eventsEl.textContent = daemon.event_count !== undefined ? daemon.event_count : '--';
}

// =============== BENCHMARK ===============

async function runBenchmark() {
    const button = document.getElementById('run-benchmark-btn');
    button.disabled = true;
    button.textContent = 'Running...';

    try {
        if (eventBus && eventBus.connected) {
            eventBus.send('benchmark', { action: 'run' });
            showNotification('Benchmark started', 'info');
        }
    } catch (error) {
        console.error('Failed to run benchmark:', error);
        showNotification('Failed to run benchmark', 'error');
    } finally {
        setTimeout(() => {
            button.disabled = false;
            button.textContent = 'Run Benchmark';
        }, 3000);
    }
}
window.runBenchmark = runBenchmark;

async function loadBenchmarkData() {
    try {
        if (eventBus && eventBus.connected) {
            eventBus.send('benchmark', { action: 'latest' });
        }
    } catch (error) {
        console.error('Failed to load benchmark data:', error);
    }
}

function renderBenchmarkData() {
    const latencyEl = document.getElementById('benchmark-latency');
    const throughputEl = document.getElementById('benchmark-throughput');
    const lastRunEl = document.getElementById('benchmark-last-run');
    const statusEl = document.getElementById('benchmark-status');

    if (!benchmarkData) {
        latencyEl.textContent = '--';
        throughputEl.textContent = '--';
        lastRunEl.textContent = 'Never';
        statusEl.textContent = 'No data available';
        return;
    }

    latencyEl.textContent = benchmarkData.avg_latency ? `${benchmarkData.avg_latency}ms` : '--';
    throughputEl.textContent = benchmarkData.throughput ? `${benchmarkData.throughput}/s` : '--';

    if (benchmarkData.timestamp) {
        const date = new Date(benchmarkData.timestamp);
        lastRunEl.textContent = date.toLocaleTimeString();
    }

    statusEl.textContent = benchmarkData.status || 'Completed';
}

// =============== TIME SERIES ===============

async function loadLatencyTimeSeries() {
    const range = document.getElementById('latency-timeseries-range').value;
    try {
        if (eventBus && eventBus.connected) {
            eventBus.send('timeseries', { series: 'latency', last: range });
        }
    } catch (error) {
        console.error('Failed to load latency time series:', error);
    }
}
window.loadLatencyTimeSeries = loadLatencyTimeSeries;

async function loadThroughputTimeSeries() {
    const range = document.getElementById('throughput-timeseries-range').value;
    try {
        if (eventBus && eventBus.connected) {
            eventBus.send('timeseries', { series: 'throughput', last: range });
        }
    } catch (error) {
        console.error('Failed to load throughput time series:', error);
    }
}
window.loadThroughputTimeSeries = loadThroughputTimeSeries;

function renderLatencyTimeSeries(data) {
    if (latencyTimeSeriesChart) {
        latencyTimeSeriesChart.destroy();
    }

    const canvas = document.getElementById('latency-timeseries-chart');
    if (!canvas || !data || !data.points) return;

    const labels = data.points.map(p => {
        const date = new Date(p.timestamp);
        return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
    });

    const values = data.points.map(p => p.value);

    latencyTimeSeriesChart = new Chart(canvas, {
        type: 'line',
        data: {
            labels: labels,
            datasets: [{
                label: 'Latency (ms)',
                data: values,
                borderColor: '#ff9500',
                backgroundColor: 'rgba(255, 149, 0, 0.1)',
                tension: 0.4,
                fill: true
            }]
        },
        options: getChartOptions('Latency')
    });
}

function renderThroughputTimeSeries(data) {
    if (throughputTimeSeriesChart) {
        throughputTimeSeriesChart.destroy();
    }

    const canvas = document.getElementById('throughput-timeseries-chart');
    if (!canvas || !data || !data.points) return;

    const labels = data.points.map(p => {
        const date = new Date(p.timestamp);
        return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
    });

    const values = data.points.map(p => p.value);

    throughputTimeSeriesChart = new Chart(canvas, {
        type: 'line',
        data: {
            labels: labels,
            datasets: [{
                label: 'Throughput (events/s)',
                data: values,
                borderColor: '#30d158',
                backgroundColor: 'rgba(48, 209, 88, 0.1)',
                tension: 0.4,
                fill: true
            }]
        },
        options: getChartOptions('Throughput')
    });
}

// =============== CLIENT STATISTICS ===============

async function loadClientStats() {
    const hostname = document.getElementById('stats-client-filter').value;
    try {
        if (eventBus && eventBus.connected) {
            eventBus.send('get_stats', { hostname: hostname });
        }
    } catch (error) {
        console.error('Failed to load client stats:', error);
    }
}
window.loadClientStats = loadClientStats;

function renderClientStats() {
    const container = document.getElementById('client-stats-grid');
    if (!container) return;

    if (!clientStats || Object.keys(clientStats).length === 0) {
        container.innerHTML = `
            <p style="grid-column: 1/-1; text-align: center; color: #86868b; padding: 20px;">
                No client statistics available yet. Stats will appear as clients connect and report.
            </p>
        `;
        return;
    }

    const cards = Object.entries(clientStats).map(([hostname, stats]) => {
        const vpnIP = stats.vpn_ip || hostnameToIP[hostname] || '--';
        const version = stats.current_version ? stats.current_version.substring(0, 8) : '--';

        return `
            <div class="client-stat-card">
                <h4>
                    ${hostname}
                    <span class="vpn-ip">${vpnIP}</span>
                </h4>
                <div class="stat-row">
                    <span class="label">Version</span>
                    <span class="value"><code>${version}</code></span>
                </div>
                <div class="stat-row">
                    <span class="label">Connects</span>
                    <span class="value">${stats.connects || 0}</span>
                </div>
                <div class="stat-row">
                    <span class="label">Disconnects</span>
                    <span class="value">${stats.disconnects || 0}</span>
                </div>
                <div class="stat-row">
                    <span class="label">Deployments</span>
                    <span class="value">${stats.deployments || 0}</span>
                </div>
                <div class="stat-row">
                    <span class="label">Successful</span>
                    <span class="value" style="color: #30d158;">${stats.successful_deploys || 0}</span>
                </div>
                ${stats.failed_deploys ? `
                <div class="stat-row">
                    <span class="label">Failed</span>
                    <span class="value" style="color: #ff3b30;">${stats.failed_deploys}</span>
                </div>
                ` : ''}
                <div class="stat-row">
                    <span class="label">Last Seen</span>
                    <span class="value">${formatLastSeen(stats.last_seen)}</span>
                </div>
            </div>
        `;
    }).join('');

    container.innerHTML = cards;
}

function formatLastSeen(lastSeen) {
    if (!lastSeen) return '--';
    try {
        const date = new Date(lastSeen);
        const now = new Date();
        const diff = now - date;

        if (diff < 60000) return 'Just now';
        if (diff < 3600000) return `${Math.floor(diff / 60000)}m ago`;
        if (diff < 86400000) return `${Math.floor(diff / 3600000)}h ago`;
        return date.toLocaleDateString();
    } catch {
        return '--';
    }
}

// =============== EVENT LOGS ===============

async function loadEventLogs() {
    const timeFilter = document.getElementById('logs-time-filter').value;
    const namespaceFilter = document.getElementById('logs-namespace-filter').value;
    const hostnameFilter = document.getElementById('logs-hostname-filter').value;
    const limit = parseInt(document.getElementById('logs-limit').value) || 100;

    try {
        if (eventBus && eventBus.connected) {
            eventBus.send('get_logs', {
                last: timeFilter,
                namespace: namespaceFilter,
                hostname: hostnameFilter,
                limit: limit
            });
        }
    } catch (error) {
        console.error('Failed to load event logs:', error);
    }
}
window.loadEventLogs = loadEventLogs;

function renderEventLogs() {
    const container = document.getElementById('logs-container');
    if (!container) return;

    if (!eventLogs || eventLogs.length === 0) {
        container.innerHTML = `
            <div style="text-align: center; color: #86868b; padding: 40px;">
                No events found matching the filters.
            </div>
        `;
        return;
    }

    const logsHTML = eventLogs.map(log => {
        const timestamp = log.ts ? new Date(log.ts).toLocaleTimeString() : '--';
        const hostname = log.hostname || '--';
        const namespace = log.ns || '--';
        const dataStr = log.data ? JSON.stringify(log.data) : '';

        return `
            <div class="log-entry">
                <span class="log-timestamp">${timestamp}</span>
                <span class="log-hostname">${hostname}</span>
                <span class="log-namespace">${namespace}</span>
                ${dataStr ? `<span class="log-data">${dataStr}</span>` : ''}
            </div>
        `;
    }).join('');

    container.innerHTML = logsHTML;
}

// =============== HOSTNAME TO IP MAPPING ===============

function renderHostnameMapping() {
    const container = document.getElementById('hostname-mapping-table');
    if (!container) return;

    const entries = Object.entries(hostnameToIP);

    if (entries.length === 0) {
        container.innerHTML = `
            <p style="text-align: center; color: #86868b; padding: 20px;">
                No hostname mappings available.
            </p>
        `;
        return;
    }

    const tableHTML = `
        <table>
            <thead>
                <tr>
                    <th>Hostname</th>
                    <th>VPN IP</th>
                    <th>Version</th>
                </tr>
            </thead>
            <tbody>
                ${entries.map(([hostname, vpnIP]) => {
                    const version = clientVersions[vpnIP] || 'unknown';
                    const shortVersion = version.length > 8 ? version.substring(0, 8) : version;
                    return `
                        <tr>
                            <td>${hostname}</td>
                            <td><code>${vpnIP}</code></td>
                            <td><code>${shortVersion}</code></td>
                        </tr>
                    `;
                }).join('')}
            </tbody>
        </table>
    `;

    container.innerHTML = tableHTML;
}

// =============== POPULATE CLIENT SELECTS ===============

function populateClientSelects() {
    // Get all client-related select elements
    const selects = [
        'update-target-select',
        'rollback-target-select',
        'logs-hostname-filter',
        'stats-client-filter'
    ];

    // Build options from peers and hostnameToIP
    const clientOptions = new Set();

    // Add from peers
    peers.forEach(peer => {
        if (peer.vpn_address) {
            clientOptions.add(peer.vpn_address);
        }
    });

    // Add from hostnameToIP
    Object.values(hostnameToIP).forEach(vpnIP => {
        if (vpnIP) {
            clientOptions.add(vpnIP);
        }
    });

    // Add from clientVersions
    Object.keys(clientVersions).forEach(vpnIP => {
        if (vpnIP) {
            clientOptions.add(vpnIP);
        }
    });

    selects.forEach(selectId => {
        const select = document.getElementById(selectId);
        if (!select) return;

        // Keep the first option (placeholder)
        const firstOption = select.options[0];
        select.innerHTML = '';
        select.appendChild(firstOption);

        // Add client options
        [...clientOptions].sort().forEach(vpnIP => {
            const option = document.createElement('option');
            option.value = vpnIP;

            // Try to find hostname for this IP
            const hostname = Object.entries(hostnameToIP).find(([h, ip]) => ip === vpnIP)?.[0];
            option.textContent = hostname ? `${hostname} (${vpnIP})` : vpnIP;

            select.appendChild(option);
        });
    });
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

    // Listen for git-updated events from main process (triggered after git pull)
    if (window.vpnAPI.onGitUpdated) {
        window.vpnAPI.onGitUpdated(() => {
            console.log('📥 Git updated notification received, reloading page...');
            showNotification('UI updated! Reloading...', 'success');
            setTimeout(() => {
                location.reload();
            }, 1000);
        });
    }

    // Also do periodic checks from renderer side
    setInterval(async () => {
        try {
            const hasUpdates = await window.vpnAPI.checkForUIUpdates();

            if (hasUpdates) {
                console.log('🔄 UI updates detected, pulling from Git...');
                const success = await window.vpnAPI.pullUIUpdates();

                if (success) {
                    console.log('✅ UI updated from Git, reloading page...');
                    await loadFeatureFlags();
                    showNotification('UI updated! Reloading...', 'success');
                    // Full page reload to pick up HTML/JS changes
                    setTimeout(() => {
                        location.reload();
                    }, 1000);
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
