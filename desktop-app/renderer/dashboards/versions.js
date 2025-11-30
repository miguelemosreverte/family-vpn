/**
 * Versions Dashboard Plugin
 *
 * Self-contained dashboard for version management and deployments.
 * Can be hot-reloaded without affecting the rest of the application.
 */

(function() {
    'use strict';

    // =============== PLUGIN STATE ===============

    let state = {
        clientVersions: {},
        serverVersion: 'unknown',
        hostnameToIP: {},
        peers: []
    };

    let versionTimelineChart = null;
    let refreshInterval = null;

    // =============== RENDER ===============

    function render() {
        return `
            <div class="version-overview">
                <div class="version-card">
                    <h3>Server Version</h3>
                    <div class="version-value" id="server-version">
                        <code>Loading...</code>
                    </div>
                    <div class="version-meta" id="server-version-meta"></div>
                </div>
                <div class="version-card">
                    <h3>Deployment Status</h3>
                    <div class="deployment-stats" id="deployment-stats">
                        <div class="stat-item">
                            <span class="stat-label">Up to date:</span>
                            <span class="stat-value status-healthy" id="up-to-date-count">0</span>
                        </div>
                        <div class="stat-item">
                            <span class="stat-label">Updating:</span>
                            <span class="stat-value status-updating" id="updating-count">0</span>
                        </div>
                        <div class="stat-item">
                            <span class="stat-label">Behind:</span>
                            <span class="stat-value status-behind" id="behind-count">0</span>
                        </div>
                    </div>
                </div>
                <div class="version-card actions-card">
                    <h3>Update Actions</h3>
                    <div class="action-buttons">
                        <button id="update-all-btn" class="action-btn primary">
                            Update All Clients
                        </button>
                        <button id="sync-versions-btn" class="action-btn secondary">
                            Sync Versions
                        </button>
                    </div>
                    <div id="update-status" class="action-status"></div>
                </div>
            </div>

            <!-- Targeted Update Section -->
            <div class="targeted-update-section">
                <div class="section-header">
                    <h3>Targeted Update</h3>
                </div>
                <div class="targeted-update-form">
                    <div class="form-group">
                        <label for="update-target-select">Target Client:</label>
                        <select id="update-target-select">
                            <option value="">Select a client...</option>
                        </select>
                    </div>
                    <div class="form-group">
                        <label for="update-version-input">Version (optional):</label>
                        <input type="text" id="update-version-input" placeholder="Leave empty for latest">
                    </div>
                    <button id="targeted-update-btn" class="action-btn">
                        Update Selected Client
                    </button>
                </div>
            </div>

            <!-- Rollback Section -->
            <div class="rollback-section">
                <div class="section-header">
                    <h3>Rollback</h3>
                </div>
                <div class="rollback-form">
                    <div class="form-group">
                        <label for="rollback-target-select">Target Client:</label>
                        <select id="rollback-target-select">
                            <option value="">Select a client...</option>
                        </select>
                    </div>
                    <div class="form-group">
                        <label for="rollback-commit-input">Rollback to Commit:</label>
                        <input type="text" id="rollback-commit-input" placeholder="Enter commit hash (required)">
                    </div>
                    <button id="rollback-btn" class="action-btn danger">
                        Rollback Client
                    </button>
                    <div id="rollback-status" class="action-status"></div>
                </div>
            </div>

            <div class="version-timeline">
                <h3>Version Timeline</h3>
                <canvas id="version-timeline-chart"></canvas>
            </div>

            <div class="clients-version">
                <div class="section-header">
                    <h3>Client Versions</h3>
                    <div class="version-controls">
                        <button id="refresh-versions-btn" class="action-btn-small">Refresh</button>
                    </div>
                </div>
                <div id="clients-version-table" class="version-table">
                    <p style="text-align: center; color: #86868b; padding: 20px;">Loading client versions...</p>
                </div>
            </div>

            <!-- Hostname to IP Mapping -->
            <div class="hostname-mapping-section">
                <div class="section-header">
                    <h3>Hostname to IP Mapping</h3>
                </div>
                <div id="hostname-mapping-table" class="mapping-table">
                    <p style="text-align: center; color: #86868b; padding: 20px;">Loading mappings...</p>
                </div>
            </div>
        `;
    }

    // =============== LIFECYCLE ===============

    function init(container, shell) {
        console.log('📦 Versions dashboard initializing...');

        // Render HTML
        container.innerHTML = render();

        // Setup event listeners
        setupEventListeners(shell);

        // Load initial data
        loadData(shell);

        // Start refresh interval
        refreshInterval = setInterval(() => loadData(shell), 30000);

        console.log('✅ Versions dashboard ready');
    }

    function destroy() {
        console.log('📦 Versions dashboard destroying...');

        // Clear interval
        if (refreshInterval) {
            clearInterval(refreshInterval);
            refreshInterval = null;
        }

        // Destroy chart
        if (versionTimelineChart) {
            versionTimelineChart.destroy();
            versionTimelineChart = null;
        }

        // Clear state
        state = {
            clientVersions: {},
            serverVersion: 'unknown',
            hostnameToIP: {},
            peers: []
        };
    }

    function onData(type, data, shell) {
        switch (type) {
            case 'client_versions':
                state.clientVersions = data.versions || {};
                if (data.server_version) {
                    state.serverVersion = data.server_version;
                }
                if (data.hostname_to_ip) {
                    state.hostnameToIP = data.hostname_to_ip;
                }
                renderVersions();
                renderHostnameMapping();
                populateClientSelects();
                break;
            case 'versions.client':
                if (data.vpn_ip && data.commit) {
                    state.clientVersions[data.vpn_ip] = data.commit;
                }
                if (data.hostname && data.vpn_ip) {
                    state.hostnameToIP[data.hostname] = data.vpn_ip;
                }
                renderVersions();
                break;
            case 'versions.server':
                if (data.version) {
                    state.serverVersion = data.version;
                }
                renderVersions();
                break;
            case 'peers':
                state.peers = data;
                populateClientSelects();
                break;
        }
    }

    // =============== DATA LOADING ===============

    async function loadData(shell) {
        try {
            // Load peers
            if (window.vpnAPI) {
                state.peers = await window.vpnAPI.getPeers();
                populateClientSelects();
            }

            // Request versions via EventBus
            if (shell && shell.eventBus && shell.eventBus.connected) {
                shell.eventBus.send('get_client_versions');
            }

            // Render with current state
            renderVersions();
            renderHostnameMapping();
        } catch (error) {
            console.error('Failed to load versions data:', error);
        }
    }

    // =============== EVENT LISTENERS ===============

    function setupEventListeners(shell) {
        // Update All button
        const updateAllBtn = document.getElementById('update-all-btn');
        if (updateAllBtn) {
            updateAllBtn.addEventListener('click', () => triggerUpdateAll(shell));
        }

        // Sync Versions button
        const syncBtn = document.getElementById('sync-versions-btn');
        if (syncBtn) {
            syncBtn.addEventListener('click', () => syncVersions(shell));
        }

        // Targeted Update button
        const targetedBtn = document.getElementById('targeted-update-btn');
        if (targetedBtn) {
            targetedBtn.addEventListener('click', () => triggerTargetedUpdate(shell));
        }

        // Rollback button
        const rollbackBtn = document.getElementById('rollback-btn');
        if (rollbackBtn) {
            rollbackBtn.addEventListener('click', () => triggerRollback(shell));
        }

        // Refresh button
        const refreshBtn = document.getElementById('refresh-versions-btn');
        if (refreshBtn) {
            refreshBtn.addEventListener('click', () => loadData(shell));
        }
    }

    // =============== RENDER FUNCTIONS ===============

    function renderVersions() {
        renderServerVersion();
        renderDeploymentStatus();
        renderClientVersionsTable();
        renderVersionTimeline();
    }

    function renderServerVersion() {
        const el = document.getElementById('server-version');
        if (el) {
            const shortVersion = state.serverVersion.length > 8 ? state.serverVersion.substring(0, 8) : state.serverVersion;
            el.innerHTML = `<code title="${state.serverVersion}">${shortVersion}</code>`;
        }
    }

    function renderDeploymentStatus() {
        const upToDateEl = document.getElementById('up-to-date-count');
        const updatingEl = document.getElementById('updating-count');
        const behindEl = document.getElementById('behind-count');

        let upToDate = 0, updating = 0, behind = 0;

        Object.values(state.clientVersions).forEach(version => {
            if (version === state.serverVersion) {
                upToDate++;
            } else if (version === 'updating') {
                updating++;
            } else {
                behind++;
            }
        });

        if (upToDateEl) upToDateEl.textContent = upToDate;
        if (updatingEl) updatingEl.textContent = updating;
        if (behindEl) behindEl.textContent = behind;
    }

    function renderClientVersionsTable() {
        const container = document.getElementById('clients-version-table');
        if (!container) return;

        const entries = Object.entries(state.clientVersions);

        if (entries.length === 0) {
            container.innerHTML = `
                <p style="text-align: center; color: #86868b; padding: 20px;">
                    No client versions available. Clients will report their versions when connected.
                </p>
            `;
            return;
        }

        const clientsList = entries.map(([vpnIP, version]) => {
            const hostname = Object.entries(state.hostnameToIP).find(([h, ip]) => ip === vpnIP)?.[0] || 'Unknown';
            const peer = state.peers.find(p => p.vpn_address === vpnIP);
            const os = peer?.os || 'Unknown';
            const isUpToDate = version === state.serverVersion;
            const isUpdating = version === 'updating';

            return {
                hostname,
                vpnIP,
                os,
                version: version.length > 8 ? version.substring(0, 8) : version,
                fullVersion: version,
                status: isUpdating ? 'Updating' : (isUpToDate ? 'Up to date' : 'Behind'),
                statusClass: isUpdating ? 'status-degraded' : (isUpToDate ? 'status-healthy' : 'status-unhealthy')
            };
        });

        container.innerHTML = `
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
                                <button class="action-btn-mini" data-vpn-ip="${client.vpnIP}">Update</button>
                            </td>
                        </tr>
                    `).join('')}
                </tbody>
            </table>
        `;

        // Add click handlers for update buttons
        container.querySelectorAll('.action-btn-mini').forEach(btn => {
            btn.addEventListener('click', () => {
                const vpnIP = btn.getAttribute('data-vpn-ip');
                updateSingleClient(vpnIP);
            });
        });
    }

    function renderVersionTimeline() {
        const canvas = document.getElementById('version-timeline-chart');
        if (!canvas) return;

        if (versionTimelineChart) {
            versionTimelineChart.destroy();
        }

        const versions = Object.entries(state.clientVersions);
        const labels = versions.map(([ip]) => {
            const hostname = Object.entries(state.hostnameToIP).find(([h, i]) => i === ip)?.[0] || ip;
            return hostname.split('.')[0]; // Short hostname
        });

        const data = versions.map(([_, v]) => v === state.serverVersion ? 1 : 0);

        const isDark = document.body.classList.contains('dark-mode');

        versionTimelineChart = new Chart(canvas, {
            type: 'bar',
            data: {
                labels,
                datasets: [{
                    label: 'Version Status',
                    data,
                    backgroundColor: data.map(d => d === 1 ? '#30d158' : '#ff3b30')
                }]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: {
                    legend: { display: false }
                },
                scales: {
                    x: {
                        grid: { color: isDark ? '#2c2c2e' : '#e5e5e7' },
                        ticks: { color: isDark ? '#86868b' : '#1d1d1f' }
                    },
                    y: {
                        display: false,
                        max: 1.5
                    }
                }
            }
        });
    }

    function renderHostnameMapping() {
        const container = document.getElementById('hostname-mapping-table');
        if (!container) return;

        const entries = Object.entries(state.hostnameToIP);

        if (entries.length === 0) {
            container.innerHTML = `
                <p style="text-align: center; color: #86868b; padding: 20px;">
                    No hostname mappings available.
                </p>
            `;
            return;
        }

        container.innerHTML = `
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
                        const version = state.clientVersions[vpnIP] || 'unknown';
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
    }

    function populateClientSelects() {
        const selects = ['update-target-select', 'rollback-target-select'];
        const clients = new Set();

        state.peers.forEach(p => p.vpn_address && clients.add(p.vpn_address));
        Object.values(state.hostnameToIP).forEach(ip => ip && clients.add(ip));
        Object.keys(state.clientVersions).forEach(ip => ip && clients.add(ip));

        const options = [...clients].sort().map(vpnIP => {
            const hostname = Object.entries(state.hostnameToIP).find(([h, ip]) => ip === vpnIP)?.[0];
            return `<option value="${vpnIP}">${hostname ? `${hostname} (${vpnIP})` : vpnIP}</option>`;
        }).join('');

        selects.forEach(id => {
            const select = document.getElementById(id);
            if (select) {
                const firstOption = select.options[0];
                select.innerHTML = '';
                select.appendChild(firstOption);
                select.insertAdjacentHTML('beforeend', options);
            }
        });
    }

    function getOSEmoji(os) {
        const osLower = (os || '').toLowerCase();
        if (osLower.includes('mac') || osLower.includes('darwin')) return '🍎';
        if (osLower.includes('linux')) return '🐧';
        if (osLower.includes('win')) return '🪟';
        return '💻';
    }

    // =============== ACTIONS ===============

    function triggerUpdateAll(shell) {
        const btn = document.getElementById('update-all-btn');
        const statusEl = document.getElementById('update-status');

        if (btn) {
            btn.disabled = true;
            btn.textContent = 'Updating...';
        }

        if (shell && shell.eventBus && shell.eventBus.connected) {
            shell.eventBus.send('broadcast', {
                ns: 'updates.available',
                data: { targets: ['all'], domain: 'all' }
            });

            if (statusEl) {
                statusEl.textContent = 'Update broadcast sent to all clients';
                statusEl.className = 'action-status success';
            }

            shell.showNotification('Update triggered for all clients', 'success');
        }

        setTimeout(() => {
            if (btn) {
                btn.disabled = false;
                btn.textContent = 'Update All Clients';
            }
        }, 2000);
    }

    function syncVersions(shell) {
        const btn = document.getElementById('sync-versions-btn');
        const statusEl = document.getElementById('update-status');

        if (btn) {
            btn.disabled = true;
            btn.textContent = 'Syncing...';
        }

        if (shell && shell.eventBus && shell.eventBus.connected) {
            shell.eventBus.send('broadcast', {
                ns: 'versions.sync',
                data: {}
            });

            if (statusEl) {
                statusEl.textContent = 'Version sync requested';
                statusEl.className = 'action-status success';
            }

            shell.showNotification('Version sync requested', 'success');
        }

        setTimeout(() => {
            if (btn) {
                btn.disabled = false;
                btn.textContent = 'Sync Versions';
            }
        }, 2000);
    }

    function triggerTargetedUpdate(shell) {
        const targetSelect = document.getElementById('update-target-select');
        const versionInput = document.getElementById('update-version-input');
        const target = targetSelect?.value;
        const version = versionInput?.value?.trim() || '';

        if (!target) {
            shell?.showNotification('Please select a target client', 'error');
            return;
        }

        const btn = document.getElementById('targeted-update-btn');
        if (btn) {
            btn.disabled = true;
            btn.textContent = 'Updating...';
        }

        if (shell && shell.eventBus && shell.eventBus.connected) {
            shell.eventBus.send('broadcast', {
                ns: 'updates.available',
                data: { targets: [target], version, domain: 'all' }
            });

            shell.showNotification(`Update triggered for ${target}`, 'success');
        }

        setTimeout(() => {
            if (btn) {
                btn.disabled = false;
                btn.textContent = 'Update Selected Client';
            }
        }, 2000);
    }

    function triggerRollback(shell) {
        const targetSelect = document.getElementById('rollback-target-select');
        const commitInput = document.getElementById('rollback-commit-input');
        const statusEl = document.getElementById('rollback-status');
        const target = targetSelect?.value;
        const commit = commitInput?.value?.trim();

        if (!target) {
            if (statusEl) {
                statusEl.textContent = 'Please select a target client';
                statusEl.className = 'action-status error';
            }
            return;
        }

        if (!commit) {
            if (statusEl) {
                statusEl.textContent = 'Please enter a commit hash';
                statusEl.className = 'action-status error';
            }
            return;
        }

        const btn = document.getElementById('rollback-btn');
        if (btn) {
            btn.disabled = true;
            btn.textContent = 'Rolling back...';
        }

        if (shell && shell.eventBus && shell.eventBus.connected) {
            shell.eventBus.send('broadcast', {
                ns: 'updates.available',
                data: { targets: [target], version: commit, rollback: true, domain: 'all' }
            });

            if (statusEl) {
                statusEl.textContent = `Rollback triggered for ${target} to ${commit.substring(0, 8)}`;
                statusEl.className = 'action-status success';
            }

            shell.showNotification('Rollback command sent', 'success');
        }

        setTimeout(() => {
            if (btn) {
                btn.disabled = false;
                btn.textContent = 'Rollback Client';
            }
        }, 2000);
    }

    function updateSingleClient(vpnIP) {
        const shell = window.dashboardShell;
        if (shell && shell.eventBus && shell.eventBus.connected) {
            shell.eventBus.send('broadcast', {
                ns: 'updates.available',
                data: { targets: [vpnIP], version: '', domain: 'all' }
            });

            shell.showNotification(`Update triggered for ${vpnIP}`, 'success');
        }
    }

    // =============== EXPORT ===============

    window.VersionsDashboard = {
        render,
        init,
        destroy,
        onData
    };

})();
