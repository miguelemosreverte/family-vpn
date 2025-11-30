/**
 * Health Dashboard Plugin
 *
 * This is a self-contained dashboard that can be hot-reloaded
 * without affecting the rest of the application.
 */

(function() {
    'use strict';

    // =============== PLUGIN STATE ===============
    // All state is scoped to this plugin

    let state = {
        pingHistory: [],
        daemonHealth: null,
        clientStats: {},
        eventLogs: [],
        benchmarkData: null,
        peers: [],
        hostnameToIP: {}
    };

    let charts = {
        pingHour: null,
        ping24h: null,
        latencyTimeSeries: null,
        throughputTimeSeries: null
    };

    let refreshInterval = null;

    // =============== RENDER ===============

    function render() {
        return `
            <!-- Daemon Status Bar -->
            <div class="daemon-status-bar" id="daemon-status-bar">
                <div class="daemon-status-item">
                    <span class="daemon-label">Daemon:</span>
                    <span class="daemon-value" id="daemon-status">Checking...</span>
                </div>
                <div class="daemon-status-item">
                    <span class="daemon-label">WebSocket:</span>
                    <span class="daemon-value" id="daemon-ws-status">--</span>
                </div>
                <div class="daemon-status-item">
                    <span class="daemon-label">Uptime:</span>
                    <span class="daemon-value" id="daemon-uptime">--</span>
                </div>
                <div class="daemon-status-item">
                    <span class="daemon-label">Reconnects:</span>
                    <span class="daemon-value" id="daemon-reconnects">--</span>
                </div>
                <div class="daemon-status-item">
                    <span class="daemon-label">Events (24h):</span>
                    <span class="daemon-value" id="daemon-events">--</span>
                </div>
            </div>

            <div class="metrics-grid">
                <div class="metric-card">
                    <h3>Connected Clients</h3>
                    <div class="metric-value" id="health-client-count">0</div>
                    <div class="metric-label">Active Connections</div>
                </div>
                <div class="metric-card">
                    <h3>Server Health</h3>
                    <div class="metric-value" id="server-health">
                        <span class="status-badge status-unknown">Unknown</span>
                    </div>
                    <div class="metric-label" id="server-latency">Latency: --</div>
                </div>
                <div class="metric-card">
                    <h3>Uptime (24h)</h3>
                    <div class="metric-value" id="uptime-24h">--%</div>
                    <div class="metric-label">Last 24 hours</div>
                </div>
                <div class="metric-card">
                    <h3>Average Latency</h3>
                    <div class="metric-value" id="avg-latency">-- ms</div>
                    <div class="metric-label">Last hour</div>
                </div>
            </div>

            <div class="charts-grid">
                <div class="chart-card">
                    <h3>Ping History - Last Hour</h3>
                    <canvas id="ping-hour-chart"></canvas>
                </div>
                <div class="chart-card">
                    <h3>Ping History - Last 24 Hours</h3>
                    <canvas id="ping-24h-chart"></canvas>
                </div>
            </div>

            <!-- Benchmark Section -->
            <div class="benchmark-section">
                <div class="section-header">
                    <h3>Performance Benchmarks</h3>
                    <button id="run-benchmark-btn" class="action-btn-small">Run Benchmark</button>
                </div>
                <div class="benchmark-grid" id="benchmark-grid">
                    <div class="benchmark-card">
                        <h4>Latency</h4>
                        <div class="benchmark-value" id="benchmark-latency">--</div>
                        <div class="benchmark-label">Average response time</div>
                    </div>
                    <div class="benchmark-card">
                        <h4>Throughput</h4>
                        <div class="benchmark-value" id="benchmark-throughput">--</div>
                        <div class="benchmark-label">Events per second</div>
                    </div>
                    <div class="benchmark-card">
                        <h4>Last Run</h4>
                        <div class="benchmark-value" id="benchmark-last-run">Never</div>
                        <div class="benchmark-label" id="benchmark-status">--</div>
                    </div>
                </div>
                <div class="timeseries-charts">
                    <div class="chart-card">
                        <h4>Latency Time Series</h4>
                        <div class="timeseries-controls">
                            <select id="latency-timeseries-range">
                                <option value="1h">Last Hour</option>
                                <option value="6h">Last 6 Hours</option>
                                <option value="24h" selected>Last 24 Hours</option>
                            </select>
                        </div>
                        <canvas id="latency-timeseries-chart"></canvas>
                    </div>
                    <div class="chart-card">
                        <h4>Throughput Time Series</h4>
                        <div class="timeseries-controls">
                            <select id="throughput-timeseries-range">
                                <option value="1h">Last Hour</option>
                                <option value="6h">Last 6 Hours</option>
                                <option value="24h" selected>Last 24 Hours</option>
                            </select>
                        </div>
                        <canvas id="throughput-timeseries-chart"></canvas>
                    </div>
                </div>
            </div>

            <!-- Client Statistics Section -->
            <div class="client-stats-section">
                <div class="section-header">
                    <h3>Client Statistics</h3>
                    <select id="stats-client-filter">
                        <option value="">All Clients</option>
                    </select>
                </div>
                <div id="client-stats-grid" class="client-stats-grid">
                    <p style="grid-column: 1/-1; text-align: center; color: #86868b; padding: 20px;">
                        Loading client statistics...
                    </p>
                </div>
            </div>

            <div class="peers-status">
                <h3>Client Status</h3>
                <div id="peers-status-grid" class="peers-grid">
                    <!-- Peer status cards will be inserted here -->
                </div>
            </div>

            <!-- Event Logs Section -->
            <div class="logs-section">
                <div class="section-header">
                    <h3>Event Logs</h3>
                    <div class="logs-controls">
                        <select id="logs-time-filter">
                            <option value="30m">Last 30 minutes</option>
                            <option value="1h">Last hour</option>
                            <option value="2h" selected>Last 2 hours</option>
                            <option value="6h">Last 6 hours</option>
                            <option value="24h">Last 24 hours</option>
                        </select>
                        <select id="logs-namespace-filter">
                            <option value="">All Namespaces</option>
                            <option value="versions">versions.*</option>
                            <option value="health">health.*</option>
                            <option value="system">system.*</option>
                            <option value="updates">updates.*</option>
                            <option value="peers">peers.*</option>
                        </select>
                        <select id="logs-hostname-filter">
                            <option value="">All Clients</option>
                        </select>
                        <input type="number" id="logs-limit" value="100" min="10" max="500" title="Max entries">
                    </div>
                </div>
                <div id="logs-container" class="logs-container">
                    <div style="text-align: center; color: #86868b; padding: 40px;">
                        Loading event logs...
                    </div>
                </div>
            </div>

            <div class="ping-history">
                <h3>Recent Ping History (Last 100)</h3>
                <div id="ping-history-table" class="ping-table">
                    <!-- Ping history table will be inserted here -->
                </div>
            </div>
        `;
    }

    // =============== LIFECYCLE ===============

    function init(container, shell) {
        console.log('🏥 Health dashboard initializing...');

        // Render HTML
        container.innerHTML = render();

        // Setup event listeners
        setupEventListeners();

        // Load initial data
        loadData(shell);

        // Start refresh interval
        refreshInterval = setInterval(() => loadData(shell), 30000);

        console.log('✅ Health dashboard ready');
    }

    function destroy() {
        console.log('🏥 Health dashboard destroying...');

        // Clear interval
        if (refreshInterval) {
            clearInterval(refreshInterval);
            refreshInterval = null;
        }

        // Destroy charts
        Object.values(charts).forEach(chart => {
            if (chart) chart.destroy();
        });
        charts = {
            pingHour: null,
            ping24h: null,
            latencyTimeSeries: null,
            throughputTimeSeries: null
        };

        // Clear state
        state = {
            pingHistory: [],
            daemonHealth: null,
            clientStats: {},
            eventLogs: [],
            benchmarkData: null,
            peers: [],
            hostnameToIP: {}
        };
    }

    function onData(type, data, shell) {
        switch (type) {
            case 'ping_history':
                state.pingHistory = data.history || [];
                renderCharts();
                renderPingHistoryTable();
                renderMetrics();
                break;
            case 'ping_update':
                state.pingHistory.push(data);
                if (state.pingHistory.length > 2880) {
                    state.pingHistory = state.pingHistory.slice(-2880);
                }
                renderCharts();
                renderPingHistoryTable();
                renderMetrics();
                break;
            case 'health':
                state.daemonHealth = data;
                renderDaemonStatus();
                break;
            case 'stats':
                if (data.clients) {
                    state.clientStats = data.clients;
                }
                renderClientStats();
                break;
            case 'logs':
                state.eventLogs = data.events || [];
                renderEventLogs();
                break;
            case 'benchmark':
                state.benchmarkData = data;
                renderBenchmarkData();
                break;
            case 'timeseries':
                if (data.series === 'latency') {
                    renderLatencyTimeSeries(data);
                } else if (data.series === 'throughput') {
                    renderThroughputTimeSeries(data);
                }
                break;
            case 'peers':
                state.peers = data;
                renderPeerStatus();
                populateClientFilters();
                break;
        }
    }

    // =============== DATA LOADING ===============

    async function loadData(shell) {
        try {
            // Load peers
            if (window.vpnAPI) {
                state.peers = await window.vpnAPI.getPeers();
                renderPeerStatus();
                populateClientFilters();
            }

            // Request data via EventBus
            if (shell && shell.eventBus && shell.eventBus.connected) {
                shell.eventBus.send('get_health');
                shell.eventBus.send('get_stats', {});
                shell.eventBus.send('get_logs', { last: '2h', limit: 100 });
                shell.eventBus.send('benchmark', { action: 'latest' });
                shell.eventBus.send('timeseries', { series: 'latency', last: '24h' });
                shell.eventBus.send('timeseries', { series: 'throughput', last: '24h' });
            }
        } catch (error) {
            console.error('Failed to load health data:', error);
        }
    }

    // =============== EVENT LISTENERS ===============

    function setupEventListeners() {
        // Benchmark button
        const benchmarkBtn = document.getElementById('run-benchmark-btn');
        if (benchmarkBtn) {
            benchmarkBtn.addEventListener('click', runBenchmark);
        }

        // Time series range selectors
        const latencyRange = document.getElementById('latency-timeseries-range');
        if (latencyRange) {
            latencyRange.addEventListener('change', () => loadTimeSeries('latency'));
        }

        const throughputRange = document.getElementById('throughput-timeseries-range');
        if (throughputRange) {
            throughputRange.addEventListener('change', () => loadTimeSeries('throughput'));
        }

        // Log filters
        ['logs-time-filter', 'logs-namespace-filter', 'logs-hostname-filter', 'logs-limit'].forEach(id => {
            const el = document.getElementById(id);
            if (el) {
                el.addEventListener('change', loadEventLogs);
            }
        });

        // Stats filter
        const statsFilter = document.getElementById('stats-client-filter');
        if (statsFilter) {
            statsFilter.addEventListener('change', loadClientStats);
        }
    }

    // =============== RENDER FUNCTIONS ===============

    function renderMetrics() {
        const clientCount = document.getElementById('health-client-count');
        const serverHealth = document.getElementById('server-health');
        const serverLatency = document.getElementById('server-latency');
        const uptime24h = document.getElementById('uptime-24h');
        const avgLatency = document.getElementById('avg-latency');

        if (clientCount) {
            clientCount.textContent = state.peers.length;
        }

        // Calculate metrics from ping history
        const now = Date.now();
        const hourAgo = now - 3600000;
        const dayAgo = now - 86400000;

        const recentPings = state.pingHistory.filter(p => p.timestamp > hourAgo);
        const dayPings = state.pingHistory.filter(p => p.timestamp > dayAgo);

        // Server health based on recent pings
        if (recentPings.length > 0) {
            const lastPing = recentPings[recentPings.length - 1];
            const successRate = recentPings.filter(p => p.success).length / recentPings.length;

            let statusClass, statusText;
            if (successRate > 0.95 && lastPing.latency < 100) {
                statusClass = 'status-healthy';
                statusText = 'Healthy';
            } else if (successRate > 0.8) {
                statusClass = 'status-degraded';
                statusText = 'Degraded';
            } else {
                statusClass = 'status-unhealthy';
                statusText = 'Unhealthy';
            }

            if (serverHealth) {
                serverHealth.innerHTML = `<span class="status-badge ${statusClass}">${statusText}</span>`;
            }
            if (serverLatency) {
                serverLatency.textContent = `Latency: ${lastPing.latency}ms`;
            }
        }

        // 24h uptime
        if (dayPings.length > 0 && uptime24h) {
            const successCount = dayPings.filter(p => p.success).length;
            const uptimePercent = ((successCount / dayPings.length) * 100).toFixed(1);
            uptime24h.textContent = `${uptimePercent}%`;
        }

        // Average latency (last hour)
        if (recentPings.length > 0 && avgLatency) {
            const avgLat = recentPings.reduce((sum, p) => sum + (p.latency || 0), 0) / recentPings.length;
            avgLatency.textContent = `${Math.round(avgLat)} ms`;
        }
    }

    function renderCharts() {
        renderPingHourChart();
        renderPing24hChart();
    }

    function renderPingHourChart() {
        const canvas = document.getElementById('ping-hour-chart');
        if (!canvas) return;

        const now = Date.now();
        const hourAgo = now - 3600000;
        const recentPings = state.pingHistory.filter(p => p.timestamp > hourAgo);

        // Group by minute
        const minuteData = {};
        recentPings.forEach(p => {
            const minute = Math.floor(p.timestamp / 60000) * 60000;
            if (!minuteData[minute]) {
                minuteData[minute] = [];
            }
            minuteData[minute].push(p.latency || 0);
        });

        const labels = [];
        const data = [];
        for (let t = hourAgo; t <= now; t += 60000) {
            const minute = Math.floor(t / 60000) * 60000;
            labels.push(new Date(minute).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }));
            const pings = minuteData[minute] || [];
            data.push(pings.length > 0 ? pings.reduce((a, b) => a + b, 0) / pings.length : null);
        }

        if (charts.pingHour) {
            charts.pingHour.destroy();
        }

        charts.pingHour = new Chart(canvas, {
            type: 'line',
            data: {
                labels,
                datasets: [{
                    label: 'Latency (ms)',
                    data,
                    borderColor: '#0a84ff',
                    backgroundColor: 'rgba(10, 132, 255, 0.1)',
                    tension: 0.4,
                    fill: true
                }]
            },
            options: getChartOptions()
        });
    }

    function renderPing24hChart() {
        const canvas = document.getElementById('ping-24h-chart');
        if (!canvas) return;

        const now = Date.now();
        const dayAgo = now - 86400000;
        const dayPings = state.pingHistory.filter(p => p.timestamp > dayAgo);

        // Group by 5-minute intervals
        const intervalData = {};
        dayPings.forEach(p => {
            const interval = Math.floor(p.timestamp / 300000) * 300000;
            if (!intervalData[interval]) {
                intervalData[interval] = [];
            }
            intervalData[interval].push(p.latency || 0);
        });

        const labels = [];
        const data = [];
        for (let t = dayAgo; t <= now; t += 300000) {
            const interval = Math.floor(t / 300000) * 300000;
            const date = new Date(interval);
            labels.push(date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }));
            const pings = intervalData[interval] || [];
            data.push(pings.length > 0 ? pings.reduce((a, b) => a + b, 0) / pings.length : null);
        }

        if (charts.ping24h) {
            charts.ping24h.destroy();
        }

        charts.ping24h = new Chart(canvas, {
            type: 'line',
            data: {
                labels,
                datasets: [{
                    label: 'Latency (ms)',
                    data,
                    borderColor: '#30d158',
                    backgroundColor: 'rgba(48, 209, 88, 0.1)',
                    tension: 0.4,
                    fill: true
                }]
            },
            options: getChartOptions()
        });
    }

    function getChartOptions() {
        const isDark = document.body.classList.contains('dark-mode');
        return {
            responsive: true,
            maintainAspectRatio: false,
            plugins: {
                legend: { display: false }
            },
            scales: {
                x: {
                    grid: { color: isDark ? '#2c2c2e' : '#e5e5e7' },
                    ticks: { color: isDark ? '#86868b' : '#1d1d1f', maxTicksLimit: 12 }
                },
                y: {
                    grid: { color: isDark ? '#2c2c2e' : '#e5e5e7' },
                    ticks: { color: isDark ? '#86868b' : '#1d1d1f' },
                    beginAtZero: true
                }
            }
        };
    }

    function renderDaemonStatus() {
        const statusEl = document.getElementById('daemon-status');
        const wsStatusEl = document.getElementById('daemon-ws-status');
        const uptimeEl = document.getElementById('daemon-uptime');
        const reconnectsEl = document.getElementById('daemon-reconnects');
        const eventsEl = document.getElementById('daemon-events');

        if (!state.daemonHealth) return;

        const daemon = state.daemonHealth.daemon || {};

        if (statusEl) {
            statusEl.textContent = daemon.connected ? 'CONNECTED' : 'DISCONNECTED';
            statusEl.className = `daemon-value ${daemon.connected ? 'connected' : 'disconnected'}`;
        }

        if (wsStatusEl) {
            wsStatusEl.textContent = daemon.connected ? 'Active' : 'Inactive';
            wsStatusEl.className = `daemon-value ${daemon.connected ? 'connected' : 'disconnected'}`;
        }

        if (uptimeEl) uptimeEl.textContent = daemon.uptime || '--';
        if (reconnectsEl) reconnectsEl.textContent = daemon.reconnects !== undefined ? daemon.reconnects : '--';
        if (eventsEl) eventsEl.textContent = daemon.event_count !== undefined ? daemon.event_count : '--';
    }

    function renderPeerStatus() {
        const container = document.getElementById('peers-status-grid');
        if (!container) return;

        if (state.peers.length === 0) {
            container.innerHTML = '<p style="grid-column: 1/-1; text-align: center; color: #86868b;">No peers connected</p>';
            return;
        }

        container.innerHTML = state.peers.map(peer => `
            <div class="peer-status-card">
                <div class="peer-info">
                    <h4>${peer.hostname || 'Unknown'}</h4>
                    <p>${peer.vpn_address || '--'}</p>
                </div>
                <div class="peer-status">
                    <span class="status-badge status-healthy">Online</span>
                </div>
            </div>
        `).join('');
    }

    function renderClientStats() {
        const container = document.getElementById('client-stats-grid');
        if (!container) return;

        const stats = state.clientStats;
        if (!stats || Object.keys(stats).length === 0) {
            container.innerHTML = `
                <p style="grid-column: 1/-1; text-align: center; color: #86868b; padding: 20px;">
                    No client statistics available yet.
                </p>
            `;
            return;
        }

        container.innerHTML = Object.entries(stats).map(([hostname, s]) => `
            <div class="client-stat-card">
                <h4>${hostname} <span class="vpn-ip">${s.vpn_ip || '--'}</span></h4>
                <div class="stat-row">
                    <span class="label">Version</span>
                    <span class="value"><code>${(s.current_version || '--').substring(0, 8)}</code></span>
                </div>
                <div class="stat-row">
                    <span class="label">Connects</span>
                    <span class="value">${s.connects || 0}</span>
                </div>
                <div class="stat-row">
                    <span class="label">Disconnects</span>
                    <span class="value">${s.disconnects || 0}</span>
                </div>
                <div class="stat-row">
                    <span class="label">Deployments</span>
                    <span class="value">${s.deployments || 0}</span>
                </div>
            </div>
        `).join('');
    }

    function renderEventLogs() {
        const container = document.getElementById('logs-container');
        if (!container) return;

        if (!state.eventLogs || state.eventLogs.length === 0) {
            container.innerHTML = `
                <div style="text-align: center; color: #86868b; padding: 40px;">
                    No events found matching the filters.
                </div>
            `;
            return;
        }

        container.innerHTML = state.eventLogs.map(log => `
            <div class="log-entry">
                <span class="log-timestamp">${log.ts ? new Date(log.ts).toLocaleTimeString() : '--'}</span>
                <span class="log-hostname">${log.hostname || '--'}</span>
                <span class="log-namespace">${log.ns || '--'}</span>
                ${log.data ? `<span class="log-data">${JSON.stringify(log.data)}</span>` : ''}
            </div>
        `).join('');
    }

    function renderBenchmarkData() {
        const latencyEl = document.getElementById('benchmark-latency');
        const throughputEl = document.getElementById('benchmark-throughput');
        const lastRunEl = document.getElementById('benchmark-last-run');
        const statusEl = document.getElementById('benchmark-status');

        if (!state.benchmarkData) return;

        if (latencyEl) latencyEl.textContent = state.benchmarkData.avg_latency ? `${state.benchmarkData.avg_latency}ms` : '--';
        if (throughputEl) throughputEl.textContent = state.benchmarkData.throughput ? `${state.benchmarkData.throughput}/s` : '--';
        if (lastRunEl && state.benchmarkData.timestamp) {
            lastRunEl.textContent = new Date(state.benchmarkData.timestamp).toLocaleTimeString();
        }
        if (statusEl) statusEl.textContent = state.benchmarkData.status || 'Completed';
    }

    function renderLatencyTimeSeries(data) {
        const canvas = document.getElementById('latency-timeseries-chart');
        if (!canvas || !data || !data.points) return;

        if (charts.latencyTimeSeries) {
            charts.latencyTimeSeries.destroy();
        }

        const labels = data.points.map(p => new Date(p.timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }));
        const values = data.points.map(p => p.value);

        charts.latencyTimeSeries = new Chart(canvas, {
            type: 'line',
            data: {
                labels,
                datasets: [{
                    label: 'Latency (ms)',
                    data: values,
                    borderColor: '#ff9500',
                    backgroundColor: 'rgba(255, 149, 0, 0.1)',
                    tension: 0.4,
                    fill: true
                }]
            },
            options: getChartOptions()
        });
    }

    function renderThroughputTimeSeries(data) {
        const canvas = document.getElementById('throughput-timeseries-chart');
        if (!canvas || !data || !data.points) return;

        if (charts.throughputTimeSeries) {
            charts.throughputTimeSeries.destroy();
        }

        const labels = data.points.map(p => new Date(p.timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }));
        const values = data.points.map(p => p.value);

        charts.throughputTimeSeries = new Chart(canvas, {
            type: 'line',
            data: {
                labels,
                datasets: [{
                    label: 'Throughput (events/s)',
                    data: values,
                    borderColor: '#30d158',
                    backgroundColor: 'rgba(48, 209, 88, 0.1)',
                    tension: 0.4,
                    fill: true
                }]
            },
            options: getChartOptions()
        });
    }

    function renderPingHistoryTable() {
        const container = document.getElementById('ping-history-table');
        if (!container) return;

        const recent = state.pingHistory.slice(-100).reverse();

        if (recent.length === 0) {
            container.innerHTML = '<p style="text-align: center; color: #86868b; padding: 20px;">No ping history available</p>';
            return;
        }

        container.innerHTML = `
            <table>
                <thead>
                    <tr>
                        <th>Time</th>
                        <th>Target</th>
                        <th>Latency</th>
                        <th>Status</th>
                    </tr>
                </thead>
                <tbody>
                    ${recent.map(p => `
                        <tr>
                            <td>${new Date(p.timestamp).toLocaleTimeString()}</td>
                            <td>${p.target || '--'}</td>
                            <td>${p.latency ? p.latency + 'ms' : '--'}</td>
                            <td><span class="status-badge ${p.success ? 'status-healthy' : 'status-unhealthy'}">${p.success ? 'OK' : 'Failed'}</span></td>
                        </tr>
                    `).join('')}
                </tbody>
            </table>
        `;
    }

    function populateClientFilters() {
        const filters = ['logs-hostname-filter', 'stats-client-filter'];
        const options = state.peers.map(p => `<option value="${p.vpn_address}">${p.hostname} (${p.vpn_address})</option>`).join('');

        filters.forEach(id => {
            const select = document.getElementById(id);
            if (select) {
                const firstOption = select.options[0];
                select.innerHTML = '';
                select.appendChild(firstOption);
                select.insertAdjacentHTML('beforeend', options);
            }
        });
    }

    // =============== ACTIONS ===============

    function runBenchmark() {
        const btn = document.getElementById('run-benchmark-btn');
        if (btn) {
            btn.disabled = true;
            btn.textContent = 'Running...';
            setTimeout(() => {
                btn.disabled = false;
                btn.textContent = 'Run Benchmark';
            }, 3000);
        }

        // Request via shell's eventBus
        if (window.dashboardShell && window.dashboardShell.eventBus) {
            window.dashboardShell.eventBus.send('benchmark', { action: 'run' });
        }
    }

    function loadTimeSeries(series) {
        const rangeEl = document.getElementById(`${series}-timeseries-range`);
        const range = rangeEl ? rangeEl.value : '24h';

        if (window.dashboardShell && window.dashboardShell.eventBus) {
            window.dashboardShell.eventBus.send('timeseries', { series, last: range });
        }
    }

    function loadEventLogs() {
        const timeFilter = document.getElementById('logs-time-filter')?.value || '2h';
        const namespaceFilter = document.getElementById('logs-namespace-filter')?.value || '';
        const hostnameFilter = document.getElementById('logs-hostname-filter')?.value || '';
        const limit = parseInt(document.getElementById('logs-limit')?.value) || 100;

        if (window.dashboardShell && window.dashboardShell.eventBus) {
            window.dashboardShell.eventBus.send('get_logs', {
                last: timeFilter,
                namespace: namespaceFilter,
                hostname: hostnameFilter,
                limit
            });
        }
    }

    function loadClientStats() {
        const hostname = document.getElementById('stats-client-filter')?.value || '';

        if (window.dashboardShell && window.dashboardShell.eventBus) {
            window.dashboardShell.eventBus.send('get_stats', { hostname });
        }
    }

    // =============== EXPORT ===============

    window.HealthDashboard = {
        render,
        init,
        destroy,
        onData
    };

})();
