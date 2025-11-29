// Family VPN Dashboard - Main App Logic

let peers = [];
let diskData = [];
let volumeData = [];

// Initialize app
async function init() {
    console.log('🚀 Family VPN Dashboard starting...');

    // Load saved theme
    const savedTheme = localStorage.getItem('theme');
    if (savedTheme === 'dark') {
        document.body.classList.add('dark-mode');
    }

    await loadData();
    setupEventListeners();
    startAutoRefresh();
}

// Load all data
async function loadData() {
    try {
        // Show loading state immediately
        showLoadingState();

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
            // Parse sizes (e.g., "108Gi" -> 108)
            const sizeNum = parseFloat(data.size);
            const usedNum = parseFloat(data.used);
            totalSize += sizeNum;
            totalUsed += usedNum;
        }
    });

    document.getElementById('total-disk').textContent =
        `${Math.round(totalUsed)}Gi / ${Math.round(totalSize)}Gi used`;
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

        loadData().finally(() => {
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

// Auto-refresh every 30 seconds
function startAutoRefresh() {
    setInterval(() => {
        console.log('🔄 Auto-refresh triggered');
        loadData();
    }, 30000); // 30 seconds
}

// Start the app when DOM is ready
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
} else {
    init();
}
