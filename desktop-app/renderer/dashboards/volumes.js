/**
 * Volumes Dashboard Plugin
 *
 * Self-contained dashboard for storage management.
 */

(function() {
    'use strict';

    let state = {
        peers: [],
        diskData: {},
        volumeData: {}
    };

    let refreshInterval = null;

    function render() {
        return `
            <div class="volumes-header">
                <div class="header-stats">
                    <span class="stat">
                        <span class="label">Connected Clients:</span>
                        <span class="value" id="client-count">0</span>
                    </span>
                    <span class="stat">
                        <span class="label">Total Disk:</span>
                        <span class="value" id="total-disk">0 GB</span>
                    </span>
                </div>
            </div>

            <section class="overview">
                <h2>Connected Clients</h2>
                <div id="clients-grid" class="clients-grid">
                    <p style="text-align: center; color: #86868b; padding: 20px;">Loading clients...</p>
                </div>
            </section>

            <section class="volumes">
                <h2>Volume Details</h2>
                <div id="volumes-container" class="volumes-container">
                    <p style="text-align: center; color: #86868b; padding: 20px;">Loading volumes...</p>
                </div>
            </section>
        `;
    }

    function init(container, shell) {
        console.log('💾 Volumes dashboard initializing...');
        container.innerHTML = render();
        loadData();
        refreshInterval = setInterval(loadData, 60000); // Refresh every minute
        console.log('✅ Volumes dashboard ready');
    }

    function destroy() {
        console.log('💾 Volumes dashboard destroying...');
        if (refreshInterval) {
            clearInterval(refreshInterval);
            refreshInterval = null;
        }
        state = { peers: [], diskData: {}, volumeData: {} };
    }

    function onData(type, data, shell) {
        if (type === 'peers') {
            state.peers = data;
            renderClients();
        }
    }

    async function loadData() {
        try {
            if (window.vpnAPI) {
                state.peers = await window.vpnAPI.getPeers();
                renderClients();
                await loadDiskData();
            }
        } catch (error) {
            console.error('Failed to load volumes data:', error);
        }
    }

    async function loadDiskData() {
        const container = document.getElementById('clients-grid');
        if (!container || state.peers.length === 0) return;

        for (const peer of state.peers) {
            try {
                const diskInfo = await window.vpnAPI.getDiskUsage(peer);
                state.diskData[peer.vpn_address] = diskInfo;
            } catch (error) {
                state.diskData[peer.vpn_address] = { error: error.message };
            }
        }

        renderClients();
        renderVolumes();
    }

    function renderClients() {
        const container = document.getElementById('clients-grid');
        const countEl = document.getElementById('client-count');
        const totalEl = document.getElementById('total-disk');

        if (!container) return;

        if (countEl) countEl.textContent = state.peers.length;

        if (state.peers.length === 0) {
            container.innerHTML = '<p style="text-align: center; color: #86868b; padding: 20px;">No clients connected</p>';
            return;
        }

        let totalSize = 0;

        container.innerHTML = state.peers.map(peer => {
            const disk = state.diskData[peer.vpn_address] || {};
            const usePercent = parseInt(disk.usePercent) || 0;
            const usageClass = usePercent > 90 ? 'danger' : usePercent > 70 ? 'warning' : '';

            if (disk.totalSize) {
                const size = parseFloat(disk.totalSize);
                totalSize += size;
            }

            return `
                <div class="client-card">
                    <div class="client-header">
                        <span class="client-name">${peer.hostname || 'Unknown'}</span>
                        <span class="client-status">${getOSEmoji(peer.os)}</span>
                    </div>
                    <div class="client-details">
                        <div class="client-detail">VPN: ${peer.vpn_address || '--'}</div>
                        <div class="client-detail">OS: ${peer.os || 'Unknown'}</div>
                    </div>
                    ${disk.totalSize ? `
                        <div class="disk-usage">
                            <h4>Disk Usage</h4>
                            <div class="usage-bar">
                                <div class="usage-fill ${usageClass}" style="width: ${usePercent}%"></div>
                            </div>
                            <div class="usage-text">
                                ${disk.totalUsed || '--'} / ${disk.totalSize || '--'} (${disk.usePercent || '--'})
                            </div>
                        </div>
                    ` : disk.error ? `
                        <div class="disk-usage">
                            <div class="usage-text" style="color: #ff3b30;">${disk.error}</div>
                        </div>
                    ` : `
                        <div class="disk-usage">
                            <div class="usage-text">Loading...</div>
                        </div>
                    `}
                </div>
            `;
        }).join('');

        if (totalEl) totalEl.textContent = `${Math.round(totalSize)} GB`;
    }

    function renderVolumes() {
        const container = document.getElementById('volumes-container');
        if (!container) return;

        const allVolumes = [];
        Object.entries(state.diskData).forEach(([vpnIP, disk]) => {
            if (disk.volumes) {
                const peer = state.peers.find(p => p.vpn_address === vpnIP);
                disk.volumes.forEach(vol => {
                    allVolumes.push({ ...vol, hostname: peer?.hostname || vpnIP });
                });
            }
        });

        if (allVolumes.length === 0) {
            container.innerHTML = '<p style="text-align: center; color: #86868b; padding: 20px;">No volume details available</p>';
            return;
        }

        container.innerHTML = allVolumes.map(vol => {
            const usePercent = parseInt(vol.usePercent) || 0;
            const usageClass = usePercent > 90 ? 'danger' : usePercent > 70 ? 'warning' : '';

            return `
                <div class="volume-card">
                    <div style="display: flex; justify-content: space-between; margin-bottom: 12px;">
                        <strong>${vol.mountPoint || vol.filesystem}</strong>
                        <span style="color: #86868b;">${vol.hostname}</span>
                    </div>
                    <div class="usage-bar">
                        <div class="usage-fill ${usageClass}" style="width: ${usePercent}%"></div>
                    </div>
                    <div style="display: flex; justify-content: space-between; margin-top: 8px; font-size: 13px; color: #86868b;">
                        <span>${vol.used || '--'} used</span>
                        <span>${vol.available || '--'} free</span>
                        <span>${vol.size || '--'} total</span>
                    </div>
                </div>
            `;
        }).join('');
    }

    function getOSEmoji(os) {
        const osLower = (os || '').toLowerCase();
        if (osLower.includes('mac') || osLower.includes('darwin')) return '🍎';
        if (osLower.includes('linux')) return '🐧';
        if (osLower.includes('win')) return '🪟';
        return '💻';
    }

    window.VolumesDashboard = { render, init, destroy, onData };
})();
