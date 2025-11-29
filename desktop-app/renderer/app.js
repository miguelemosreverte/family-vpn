// Family VPN Dashboard - Main App Logic

let peers = [];
let diskData = [];
let volumeData = [];
let featureFlags = {};

// Initialize app
async function init() {
    console.log('🚀 Family VPN Dashboard starting...');

    // Load feature flags
    await loadFeatureFlags();

    // Load saved theme
    const savedTheme = localStorage.getItem('theme');
    if (savedTheme === 'dark') {
        document.body.classList.add('dark-mode');
    }

    await loadData(true); // Show loading on initial load
    setupEventListeners();
    startAutoRefresh();

    // Start hot-reload watcher if enabled
    if (featureFlags.features?.autoUpdate?.enabled) {
        startHotReload();
    }

    // Add hidden update button listener
    setupUpdateHotkey();
}

// Load feature flags from Git-tracked JSON file
async function loadFeatureFlags() {
    try {
        featureFlags = await window.vpnAPI.getFeatureFlags();
        console.log('✅ Feature flags loaded:', featureFlags);

        // Apply feature flags to UI
        applyFeatureFlags();
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

// Apply feature flags to UI
function applyFeatureFlags() {
    const flags = featureFlags.features || {};

    // Example: Hide/show features based on flags
    if (!flags.darkMode?.enabled) {
        const themeToggle = document.getElementById('theme-toggle');
        if (themeToggle) themeToggle.style.display = 'none';
    }

    // Apply UI settings
    if (featureFlags.ui?.refreshInterval) {
        console.log(`🔧 Setting refresh interval to ${featureFlags.ui.refreshInterval}ms`);
    }
}

// Hot-reload: Check Git for UI updates without restarting app
function startHotReload() {
    const checkInterval = (featureFlags.features?.autoUpdate?.checkIntervalMinutes || 5) * 60 * 1000;

    console.log(`🔥 Hot-reload enabled (checking every ${checkInterval/60000} minutes)`);

    setInterval(async () => {
        try {
            const hasUpdates = await window.vpnAPI.checkForUIUpdates();

            if (hasUpdates) {
                console.log('🔄 UI updates detected, pulling from Git...');
                const success = await window.vpnAPI.pullUIUpdates();

                if (success) {
                    console.log('✅ UI updated from Git, reloading...');

                    // Reload feature flags
                    await loadFeatureFlags();

                    // Reload CSS without full page reload
                    reloadCSS();

                    // Show notification
                    showNotification('UI updated from Git!', 'success');
                }
            }
        } catch (error) {
            console.error('❌ Hot-reload check failed:', error);
        }
    }, checkInterval);
}

// Reload CSS files without full page reload
function reloadCSS() {
    const links = document.querySelectorAll('link[rel="stylesheet"]');
    links.forEach(link => {
        const href = link.getAttribute('href').split('?')[0];
        link.setAttribute('href', href + '?reload=' + new Date().getTime());
    });
    console.log('🎨 CSS reloaded');
}

// Setup hidden update hotkey (Cmd+Shift+U)
function setupUpdateHotkey() {
    document.addEventListener('keydown', async (e) => {
        // Cmd+Shift+U on macOS (or Ctrl+Shift+U on other platforms)
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

// Show notification banner
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

    // Auto-remove after 5 seconds
    setTimeout(() => {
        banner.style.opacity = '0';
        banner.style.transition = 'opacity 0.3s';
        setTimeout(() => banner.remove(), 300);
    }, 5000);
}

// Load all data
async function loadData(showLoading = false) {
    try {
        // Only show loading state for manual refresh, not auto-refresh
        if (showLoading) {
            showLoadingState();
        }

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

        // Render UI
        renderClients();
        renderVolumes();
        updateStats();
        updateTimestamp();

    } catch (error) {
        console.error('❌ Failed to load data:', error);
        showError('Failed to load VPN data. Make sure VPN is connected.');
    }
}

// Show loading state
function showLoadingState() {
    const grid = document.getElementById('clients-grid');
    grid.innerHTML = `
        <div class="loading-message">
            <h3>⏳ Loading VPN clients...</h3>
            <p>Connecting to peers via SSH...</p>
        </div>
    `;
}

// Render client cards
function renderClients() {
    const grid = document.getElementById('clients-grid');
    grid.innerHTML = '';

    diskData.forEach(data => {
        if (data.error) {
            grid.innerHTML += createErrorCard(data);
            return;
        }

        const card = document.createElement('div');
        card.className = 'client-card';

        const usedPercent = parseInt(data.usePercent);
        const statusClass = usedPercent > 90 ? 'critical' : usedPercent > 75 ? 'warning' : 'healthy';

        card.innerHTML = `
            <div class="card-header">
                <h3>${data.peer}</h3>
                <span class="status ${statusClass}">${usedPercent}%</span>
            </div>
            <div class="card-body">
                <div class="disk-info">
                    <div class="info-row">
                        <span class="label">VPN Address</span>
                        <span class="value">${data.vpn_address}</span>
                    </div>
                    <div class="info-row">
                        <span class="label">Total Capacity</span>
                        <span class="value">${data.totalSize}</span>
                    </div>
                    <div class="info-row">
                        <span class="label">Used</span>
                        <span class="value">${data.totalUsed}</span>
                    </div>
                    <div class="info-row">
                        <span class="label">Available</span>
                        <span class="value">${data.totalAvailable}</span>
                    </div>
                    <div class="info-row">
                        <span class="label">Physical Volumes</span>
                        <span class="value">${data.volumeCount || 1}</span>
                    </div>
                </div>
                <div class="progress-bar">
                    <div class="progress-fill ${statusClass}" style="width: ${usedPercent}%"></div>
                </div>
            </div>
        `;

        grid.appendChild(card);
    });
}

// Render volume details
function renderVolumes() {
    const container = document.getElementById('volumes-container');
    container.innerHTML = '';

    volumeData.forEach(data => {
        if (data.error) return;

        const section = document.createElement('div');
        section.className = 'volume-section';

        section.innerHTML = `
            <h3>📁 ${data.peer} (${data.vpn_address})</h3>
            <table class="volumes-table">
                <thead>
                    <tr>
                        <th>Filesystem</th>
                        <th>Size</th>
                        <th>Used</th>
                        <th>Available</th>
                        <th>Use%</th>
                        <th>Mounted On</th>
                    </tr>
                </thead>
                <tbody>
                    ${data.volumes.map(vol => `
                        <tr>
                            <td>${vol.filesystem}</td>
                            <td>${vol.size}</td>
                            <td>${vol.used}</td>
                            <td>${vol.available}</td>
                            <td>
                                <span class="use-percent ${getStatusClass(vol.usePercent)}">
                                    ${vol.usePercent}
                                </span>
                            </td>
                            <td>${vol.mountPoint}</td>
                        </tr>
                    `).join('')}
                </tbody>
            </table>
        `;

        container.appendChild(section);
    });
}

// Update statistics
function updateStats() {
    let totalSize = 0;
    let totalUsed = 0;

    diskData.forEach(data => {
        if (!data.error) {
            // Parse sizes (e.g., "1369Gi" -> 1369)
            const sizeNum = parseFloat(data.totalSize);
            const usedNum = parseFloat(data.totalUsed);
            totalSize += sizeNum;
            totalUsed += usedNum;
        }
    });

    const usedPercent = totalSize > 0 ? Math.round((totalUsed / totalSize) * 100) : 0;
    document.getElementById('total-disk').textContent =
        `${Math.round(totalUsed)}Gi / ${Math.round(totalSize)}Gi (${usedPercent}%)`;
}

// Update timestamp
function updateTimestamp() {
    const now = new Date().toLocaleTimeString();
    document.getElementById('last-updated').textContent = now;
}

// Get status class for percentage
function getStatusClass(percent) {
    const num = parseInt(percent);
    if (num > 90) return 'critical';
    if (num > 75) return 'warning';
    return 'healthy';
}

// Create error card
function createErrorCard(data) {
    return `
        <div class="client-card error">
            <div class="card-header">
                <h3>⚠️ ${data.peer}</h3>
                <span class="status critical">Error</span>
            </div>
            <div class="card-body">
                <p class="error-message">${data.error}</p>
            </div>
        </div>
    `;
}

// Show error message
function showError(message) {
    const grid = document.getElementById('clients-grid');
    grid.innerHTML = `
        <div class="error-banner">
            <h3>⚠️ Error</h3>
            <p>${message}</p>
        </div>
    `;
}

// Setup event listeners
function setupEventListeners() {
    const refreshBtn = document.getElementById('refresh-btn');

    refreshBtn.addEventListener('click', () => {
        console.log('🔄 Manual refresh triggered');
        // Immediate visual feedback
        refreshBtn.disabled = true;
        refreshBtn.textContent = '⏳ Loading...';
        refreshBtn.classList.add('loading');

        loadData(true).finally(() => { // Show loading on manual refresh
            refreshBtn.disabled = false;
            refreshBtn.textContent = '🔄 Refresh';
            refreshBtn.classList.remove('loading');
        });
    });

    // Theme toggle
    document.getElementById('theme-toggle').addEventListener('click', () => {
        document.body.classList.toggle('dark-mode');
        const isDark = document.body.classList.contains('dark-mode');
        localStorage.setItem('theme', isDark ? 'dark' : 'light');
        console.log(`🎨 Theme switched to ${isDark ? 'dark' : 'light'} mode`);
    });
}

// Auto-refresh every 30 seconds (silent, no loading screen)
function startAutoRefresh() {
    setInterval(() => {
        console.log('🔄 Auto-refresh triggered (background)');
        loadData(false); // Silent refresh - don't clear the screen
    }, 30000); // 30 seconds
}

// Start the app when DOM is ready
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
} else {
    init();
}
