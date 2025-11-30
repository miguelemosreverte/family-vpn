/**
 * Dashboard Shell - The Stable Core
 *
 * This file handles:
 * - Navigation between dashboards
 * - WebSocket connection to server
 * - Dynamic plugin loading and hot-reloading
 * - Theme management
 *
 * Dashboards are loaded as plugins and can be hot-reloaded
 * without affecting the shell or other dashboards.
 */

(function() {
    'use strict';

    // =============== SHELL STATE ===============

    const shell = {
        currentDashboard: 'health',
        currentPlugin: null,
        eventBus: null,
        pluginVersions: {},
        wsConnected: false
    };

    // Dashboard titles
    const DASHBOARD_TITLES = {
        'home': 'Home',
        'health': 'Health Monitoring',
        'volumes': 'Storage Management',
        'versions': 'Version Control'
    };

    // Dashboard plugin files
    const DASHBOARD_PLUGINS = {
        'health': 'dashboards/health.js',
        'versions': 'dashboards/versions.js',
        'volumes': 'dashboards/volumes.js',
        'home': 'dashboards/home.js'
    };

    // Expose shell globally for plugins
    window.dashboardShell = shell;

    // =============== INITIALIZATION ===============

    async function init() {
        console.log('🚀 Dashboard shell starting...');

        // Load saved theme
        loadTheme();

        // Setup event listeners
        setupEventListeners();

        // Connect to WebSocket
        connectWebSocket();

        // Load initial dashboard
        await switchDashboard(shell.currentDashboard);

        // Start hot-reload checker
        startHotReloadChecker();

        console.log('✅ Dashboard shell ready');
    }

    // =============== THEME ===============

    function loadTheme() {
        const savedTheme = localStorage.getItem('theme');
        if (savedTheme === 'dark') {
            document.body.classList.add('dark-mode');
        }
    }

    function toggleTheme() {
        document.body.classList.toggle('dark-mode');
        const isDark = document.body.classList.contains('dark-mode');
        localStorage.setItem('theme', isDark ? 'dark' : 'light');
    }

    // =============== EVENT LISTENERS ===============

    function setupEventListeners() {
        // Navigation
        document.querySelectorAll('.nav-item').forEach(item => {
            item.addEventListener('click', () => {
                const dashboard = item.getAttribute('data-dashboard');
                switchDashboard(dashboard);
            });
        });

        // Theme toggle
        const themeToggle = document.getElementById('theme-toggle');
        if (themeToggle) {
            themeToggle.addEventListener('click', toggleTheme);
        }

        // Refresh button
        const refreshBtn = document.getElementById('refresh-btn');
        if (refreshBtn) {
            refreshBtn.addEventListener('click', () => {
                if (shell.currentPlugin && shell.currentPlugin.init) {
                    const container = document.getElementById('dashboard-content');
                    shell.currentPlugin.destroy?.();
                    shell.currentPlugin.init(container, shell);
                }
            });
        }

        // Listen for git updates from main process
        if (window.vpnAPI && window.vpnAPI.onGitUpdated) {
            window.vpnAPI.onGitUpdated(() => {
                console.log('📥 Git updated, reloading current plugin...');
                reloadCurrentPlugin();
            });
        }
    }

    // =============== NAVIGATION ===============

    async function switchDashboard(dashboard) {
        console.log(`🔄 Switching to dashboard: ${dashboard}`);

        // Update nav items
        document.querySelectorAll('.nav-item').forEach(item => {
            item.classList.remove('active');
            if (item.getAttribute('data-dashboard') === dashboard) {
                item.classList.add('active');
            }
        });

        // Update title
        const titleEl = document.getElementById('dashboard-title');
        if (titleEl) {
            titleEl.textContent = DASHBOARD_TITLES[dashboard] || dashboard;
        }

        // Destroy current plugin
        if (shell.currentPlugin && shell.currentPlugin.destroy) {
            shell.currentPlugin.destroy();
        }

        // Load new plugin
        shell.currentDashboard = dashboard;
        await loadPlugin(dashboard);

        // Update last updated
        updateLastUpdated();
    }

    // =============== PLUGIN LOADING ===============

    async function loadPlugin(name) {
        const container = document.getElementById('dashboard-content');
        if (!container) return;

        // Show loading
        container.innerHTML = '<div class="loading-message"><p>Loading dashboard...</p></div>';

        const pluginFile = DASHBOARD_PLUGINS[name];
        if (!pluginFile) {
            container.innerHTML = `<div class="welcome-message"><h1>Welcome to Family VPN</h1><p>Select a dashboard from the sidebar.</p></div>`;
            shell.currentPlugin = null;
            return;
        }

        try {
            // Load plugin script with cache-busting
            await loadScript(pluginFile + '?v=' + Date.now());

            // Get plugin from global scope
            const pluginName = name.charAt(0).toUpperCase() + name.slice(1) + 'Dashboard';
            const plugin = window[pluginName];

            if (plugin && plugin.init) {
                shell.currentPlugin = plugin;
                plugin.init(container, shell);
                console.log(`✅ Plugin ${name} loaded`);
            } else {
                throw new Error(`Plugin ${pluginName} not found`);
            }
        } catch (error) {
            console.error(`Failed to load plugin ${name}:`, error);
            container.innerHTML = `
                <div class="error-message" style="text-align: center; padding: 40px;">
                    <h3>Failed to load dashboard</h3>
                    <p>${error.message}</p>
                    <button onclick="location.reload()" class="action-btn" style="margin-top: 20px;">Reload</button>
                </div>
            `;
        }
    }

    function loadScript(src) {
        return new Promise((resolve, reject) => {
            // Remove old script if exists
            const existingScript = document.querySelector(`script[src^="${src.split('?')[0]}"]`);
            if (existingScript) {
                existingScript.remove();
            }

            const script = document.createElement('script');
            script.src = src;
            script.onload = resolve;
            script.onerror = reject;
            document.body.appendChild(script);
        });
    }

    async function reloadCurrentPlugin() {
        if (shell.currentDashboard) {
            showNotification('Updating dashboard...', 'info');
            await loadPlugin(shell.currentDashboard);
            showNotification('Dashboard updated!', 'success');
        }
    }

    // =============== HOT RELOAD ===============

    function startHotReloadChecker() {
        // Check for plugin updates every 30 seconds
        setInterval(async () => {
            try {
                // Check if git has new changes
                if (window.vpnAPI && window.vpnAPI.checkForUIUpdates) {
                    const hasUpdates = await window.vpnAPI.checkForUIUpdates();
                    if (hasUpdates) {
                        console.log('🔄 UI updates detected, pulling...');
                        const success = await window.vpnAPI.pullUIUpdates();
                        if (success) {
                            // Reload only the current plugin, not the whole page
                            reloadCurrentPlugin();
                        }
                    }
                }
            } catch (error) {
                console.error('Hot-reload check failed:', error);
            }
        }, 30000);
    }

    // =============== WEBSOCKET ===============

    function connectWebSocket() {
        if (!window.EventBus) {
            console.error('EventBus not available');
            return;
        }

        const { createEventBus, SUB } = window.EventBus;

        // Determine WebSocket URL
        const wsUrl = getWebSocketUrl();
        console.log(`🔌 Connecting to WebSocket: ${wsUrl}`);

        shell.eventBus = createEventBus({
            url: wsUrl,
            subscriptions: ['*'], // Subscribe to all events
            onConnect: () => {
                console.log('✅ WebSocket connected');
                shell.wsConnected = true;
                updateConnectionStatus(true);

                // Request initial data
                shell.eventBus.send('get_client_versions');
            },
            onDisconnect: () => {
                console.log('❌ WebSocket disconnected');
                shell.wsConnected = false;
                updateConnectionStatus(false);
            },
            onSnapshot: (snapshot) => {
                console.log('📸 Received snapshot:', snapshot);
                handleSnapshot(snapshot);
            },
            onError: (error) => {
                console.error('WebSocket error:', error);
            }
        });

        // Subscribe to all events and route to current plugin
        shell.eventBus.subscribe('*', (event) => {
            handleEvent(event);
        });

        shell.eventBus.connect();
    }

    function getWebSocketUrl() {
        // Try to get VPN IP from peers file or use default
        const defaultUrl = 'ws://10.8.0.1:9000/ws';

        // Get local VPN IP if available
        if (window.vpnAPI) {
            // We'll use a placeholder, the actual IP will be determined
            return defaultUrl + '?vpn_ip=dashboard';
        }

        return defaultUrl;
    }

    function updateConnectionStatus(connected) {
        const indicator = document.getElementById('ws-status');
        const text = document.getElementById('ws-status-text');
        const footer = document.getElementById('ws-connection');

        if (indicator) {
            indicator.classList.toggle('connected', connected);
        }

        if (text) {
            text.textContent = connected ? 'Connected' : 'Disconnected';
        }

        if (footer) {
            footer.textContent = connected ? 'Connected' : 'Disconnected';
        }
    }

    function handleSnapshot(snapshot) {
        // Route snapshot data to current plugin
        if (shell.currentPlugin && shell.currentPlugin.onData) {
            if (snapshot.state) {
                Object.entries(snapshot.state).forEach(([key, value]) => {
                    shell.currentPlugin.onData(key, value, shell);
                });
            }
        }
    }

    function handleEvent(event) {
        // Convert event namespace to type for plugin routing
        const type = eventToType(event);

        // Route to current plugin
        if (shell.currentPlugin && shell.currentPlugin.onData) {
            shell.currentPlugin.onData(type, event.data, shell);
        }

        // Also handle some events at shell level
        handleShellEvent(type, event.data);
    }

    function eventToType(event) {
        // Convert event namespace to a simple type
        // e.g., "health.ping" -> "ping_update", "versions.client" -> "versions.client"
        const ns = event.ns || '';

        // Map common event types
        const typeMap = {
            'health.ping': 'ping_update',
            'system.snapshot': 'snapshot'
        };

        return typeMap[ns] || ns;
    }

    function handleShellEvent(type, data) {
        // Handle events that affect the shell (not just plugins)
        switch (type) {
            case 'system.connect':
                console.log('🔌 System connected:', data);
                break;
            case 'system.disconnect':
                console.log('🔌 System disconnected:', data);
                break;
        }
    }

    // =============== UTILITIES ===============

    function updateLastUpdated() {
        const el = document.getElementById('last-updated');
        if (el) {
            el.textContent = new Date().toLocaleTimeString();
        }
    }

    function showNotification(message, type = 'info') {
        const container = document.getElementById('notification-container') || document.body;

        const notification = document.createElement('div');
        notification.className = `notification notification-${type}`;
        notification.textContent = message;
        notification.style.cssText = `
            position: fixed;
            top: 20px;
            right: 20px;
            padding: 12px 20px;
            border-radius: 8px;
            color: white;
            font-size: 14px;
            z-index: 10000;
            animation: slideIn 0.3s ease;
            background: ${type === 'success' ? '#30d158' : type === 'error' ? '#ff3b30' : '#0a84ff'};
        `;

        container.appendChild(notification);

        setTimeout(() => {
            notification.style.animation = 'slideOut 0.3s ease';
            setTimeout(() => notification.remove(), 300);
        }, 3000);
    }

    // Expose showNotification for plugins
    shell.showNotification = showNotification;

    // =============== START ===============

    // Initialize when DOM is ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }

})();
