/**
 * Update Protocol for Family VPN - JavaScript Implementation
 *
 * This is Layer 0 - the foundation that all other components build upon.
 * Mirror of protocol/update.go for use in Electron/browser contexts.
 *
 * Design Principles:
 * 1. VPN connection is sacred - never restart unless core binary changes
 * 2. WebSocket is the transport - all notifications flow through it
 * 3. Git is the source of truth - versions are commits
 * 4. Components are independent - each domain handles its own updates
 */

// Domain identifies which component should handle the update
const Domain = {
  ALL: 'all',           // Broadcasts to all components (use sparingly)
  CORE: 'core',         // VPN tunnel binary - requires restart
  SERVER: 'server',     // VPN server - handled server-side
  DESKTOP: 'desktop',   // Electron shell - may require app restart
  UI: 'ui',             // Renderer HTML/CSS/JS - hot-reloads without restart
  MENUBAR: 'menubar',   // Menu bar app - can self-update
  EXTENSION: 'extension' // Specific extension - reloads via IPC
};

// Action defines what the receiver should do
const Action = {
  RELOAD: 'reload',     // Pull latest and reload (no restart)
  RESTART: 'restart',   // Pull latest and restart the component
  NOTIFY: 'notify'      // Just show a notification, user decides
};

/**
 * UpdateMessage is the canonical format for all update notifications.
 * This is the artifact - it will not change.
 */
class UpdateMessage {
  constructor(domain, action) {
    this.v = 1;                    // Protocol version
    this.domain = domain;          // Which component should handle this
    this.action = action;          // What to do
    this.commit = null;            // Git commit hash (optional)
    this.target = null;            // Extension target (optional)
    this.files = null;             // Changed files list (optional)
    this.ts = new Date().toISOString(); // Timestamp
    this.msg = null;               // Human-readable message (optional)
  }

  withCommit(commit) {
    this.commit = commit;
    return this;
  }

  withTarget(target) {
    this.target = target;
    return this;
  }

  withFiles(files) {
    this.files = files;
    return this;
  }

  withMessage(msg) {
    this.msg = msg;
    return this;
  }

  toJSON() {
    const obj = { v: this.v, domain: this.domain, action: this.action, ts: this.ts };
    if (this.commit) obj.commit = this.commit;
    if (this.target) obj.target = this.target;
    if (this.files) obj.files = this.files;
    if (this.msg) obj.msg = this.msg;
    return JSON.stringify(obj);
  }

  /**
   * Returns true if this update requires process restart
   */
  requiresRestart() {
    return this.action === Action.RESTART || this.domain === Domain.CORE;
  }

  /**
   * Returns true if this receiver should handle the message
   */
  shouldHandle(receiverDomain, receiverTarget = '') {
    // All domain matches everyone
    if (this.domain === Domain.ALL) {
      return true;
    }

    // Direct domain match
    if (this.domain === receiverDomain) {
      // For extensions, also check target
      if (this.domain === Domain.EXTENSION && this.target) {
        return this.target === receiverTarget;
      }
      return true;
    }

    return false;
  }
}

/**
 * Parse an update message from JSON
 */
function parseUpdateMessage(jsonString) {
  const data = typeof jsonString === 'string' ? JSON.parse(jsonString) : jsonString;

  const msg = new UpdateMessage(data.domain, data.action);
  msg.v = data.v || 1;
  msg.commit = data.commit || null;
  msg.target = data.target || null;
  msg.files = data.files || null;
  msg.ts = data.ts || new Date().toISOString();
  msg.msg = data.msg || null;

  return msg;
}

/**
 * Create a new update message
 */
function newUpdateMessage(domain, action) {
  return new UpdateMessage(domain, action);
}

// Export for both Node.js and browser
if (typeof module !== 'undefined' && module.exports) {
  module.exports = { Domain, Action, UpdateMessage, parseUpdateMessage, newUpdateMessage };
} else if (typeof window !== 'undefined') {
  window.UpdateProtocol = { Domain, Action, UpdateMessage, parseUpdateMessage, newUpdateMessage };
}

/*
 * Example usage:
 *
 * // Creating a message (server-side or for testing)
 * const msg = newUpdateMessage(Domain.UI, Action.RELOAD)
 *   .withCommit('abc123')
 *   .withFiles(['renderer/app.js', 'renderer/style.css']);
 * ws.send(msg.toJSON());
 *
 * // Receiving a message (client-side)
 * ws.onmessage = (event) => {
 *   const msg = parseUpdateMessage(event.data);
 *   if (msg.shouldHandle(Domain.UI)) {
 *     if (msg.requiresRestart()) {
 *       location.reload();
 *     } else {
 *       hotReloadCSS();
 *       hotReloadJS(msg.files);
 *     }
 *   }
 * };
 */
