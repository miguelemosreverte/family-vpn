/**
 * Home Dashboard Plugin
 */

(function() {
    'use strict';

    function render() {
        return `
            <div class="welcome-message">
                <h1>Welcome to Family VPN</h1>
                <p>Select a dashboard from the sidebar to get started.</p>
                <div style="margin-top: 40px; display: grid; grid-template-columns: repeat(3, 1fr); gap: 20px; max-width: 600px; margin-left: auto; margin-right: auto;">
                    <div class="metric-card" style="cursor: pointer;" onclick="document.querySelector('[data-dashboard=health]').click()">
                        <h3>Health</h3>
                        <div style="font-size: 48px;">💚</div>
                        <p style="color: #86868b; font-size: 13px;">Monitor system health</p>
                    </div>
                    <div class="metric-card" style="cursor: pointer;" onclick="document.querySelector('[data-dashboard=volumes]').click()">
                        <h3>Storage</h3>
                        <div style="font-size: 48px;">💾</div>
                        <p style="color: #86868b; font-size: 13px;">Manage disk volumes</p>
                    </div>
                    <div class="metric-card" style="cursor: pointer;" onclick="document.querySelector('[data-dashboard=versions]').click()">
                        <h3>Versions</h3>
                        <div style="font-size: 48px;">📦</div>
                        <p style="color: #86868b; font-size: 13px;">Track deployments</p>
                    </div>
                </div>
            </div>
        `;
    }

    function init(container, shell) {
        container.innerHTML = render();
    }

    function destroy() {}

    function onData(type, data, shell) {}

    window.HomeDashboard = { render, init, destroy, onData };
})();
