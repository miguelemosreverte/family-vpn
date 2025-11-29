/**
 * Event Bus Client for Family VPN - JavaScript Implementation
 *
 * This is Layer 0 - the foundational event system for all UI components.
 * Mirror of protocol/events.go for use in Electron/browser contexts.
 *
 * Architecture:
 *
 *   ┌─────────────────────────────────────────────────────────────────┐
 *   │                      EventBus Client                            │
 *   ├─────────────────────────────────────────────────────────────────┤
 *   │  Features:                                                      │
 *   │    - Subscribe to namespaced events (e.g., "versions.*")       │
 *   │    - Receive snapshots for initial state                       │
 *   │    - Automatic reconnection with backoff                       │
 *   │    - Event replay from last sequence on reconnect              │
 *   └─────────────────────────────────────────────────────────────────┘
 *
 * Event Format:
 *
 *   {
 *     "ns": "versions.client_updated",
 *     "ts": 1234567890123,
 *     "seq": 42,
 *     "data": { ... }
 *   }
 */

// Namespace constants for type safety
const NS = {
  // System events
  SYSTEM_CONNECT: 'system.connect',
  SYSTEM_DISCONNECT: 'system.disconnect',
  SYSTEM_SNAPSHOT: 'system.snapshot',
  SYSTEM_ERROR: 'system.error',

  // Health events
  HEALTH_PING: 'health.ping',
  HEALTH_LATENCY: 'health.latency',
  HEALTH_UPTIME: 'health.uptime',

  // Version events
  VERSIONS_CLIENT: 'versions.client',
  VERSIONS_SERVER: 'versions.server',
  VERSIONS_SYNC: 'versions.sync',

  // Update events
  UPDATES_AVAILABLE: 'updates.available',
  UPDATES_APPLIED: 'updates.applied',
  UPDATES_REJECTED: 'updates.rejected',

  // Peer events
  PEERS_JOINED: 'peers.joined',
  PEERS_LEFT: 'peers.left',
  PEERS_UPDATED: 'peers.updated',

  // Dashboard events
  DASHBOARD_REFRESH: 'dashboard.refresh'
};

// Predefined subscription patterns
const SUB = {
  ALL: ['*'],
  VERSIONS: ['versions.*'],
  HEALTH: ['health.*'],
  PEERS: ['peers.*'],
  UPDATES: ['updates.*']
};

/**
 * Event represents a namespaced event from the server.
 */
class Event {
  constructor(data) {
    this.ns = data.ns;
    this.ts = data.ts;
    this.seq = data.seq;
    this.data = data.data;
    this.sid = data.sid || null;
  }

  /**
   * Check if this event matches a namespace pattern.
   */
  matches(pattern) {
    return matchPattern(pattern, this.ns);
  }

  /**
   * Get namespace parts (e.g., ['versions', 'client'] for 'versions.client')
   */
  parts() {
    return this.ns.split('.');
  }

  /**
   * Get namespace domain (first part)
   */
  domain() {
    return this.parts()[0];
  }

  /**
   * Get event type (second part)
   */
  type() {
    const p = this.parts();
    return p.length > 1 ? p[1] : null;
  }
}

/**
 * Snapshot represents consolidated state at a point in time.
 */
class Snapshot {
  constructor(data) {
    this.id = data.id;
    this.ts = data.ts;
    this.state = data.state || {};
    this.lastSeq = data.last_seq || 0;
  }

  /**
   * Get state for a specific namespace.
   */
  get(namespace) {
    return this.state[namespace];
  }

  /**
   * Check if snapshot has state for a namespace.
   */
  has(namespace) {
    return namespace in this.state;
  }
}

/**
 * EventBus client handles WebSocket connection and event subscriptions.
 */
class EventBusClient {
  constructor(options = {}) {
    this.url = options.url || null;
    this.subscriptions = options.subscriptions || ['*'];
    this.autoReconnect = options.autoReconnect !== false;
    this.reconnectDelay = options.reconnectDelay || 1000;
    this.maxReconnectDelay = options.maxReconnectDelay || 30000;

    this.ws = null;
    this.handlers = new Map(); // pattern -> [handlers]
    this.lastSeq = 0;
    this.currentSnapshot = null;
    this.reconnectAttempts = 0;
    this.connected = false;

    // Lifecycle callbacks
    this.onConnect = options.onConnect || (() => {});
    this.onDisconnect = options.onDisconnect || (() => {});
    this.onSnapshot = options.onSnapshot || (() => {});
    this.onError = options.onError || ((err) => console.error('[EventBus]', err));
  }

  /**
   * Connect to the WebSocket server.
   */
  connect(url) {
    if (url) this.url = url;
    if (!this.url) {
      throw new Error('EventBus: No WebSocket URL provided');
    }

    try {
      this.ws = new WebSocket(this.url);
      this._setupHandlers();
    } catch (err) {
      this.onError(err);
      this._scheduleReconnect();
    }
  }

  /**
   * Disconnect from the server.
   */
  disconnect() {
    this.autoReconnect = false;
    if (this.ws) {
      this.ws.close();
    }
  }

  /**
   * Subscribe to a namespace pattern.
   * @param {string} pattern - e.g., "versions.*", "health.ping"
   * @param {function} handler - Called with Event object
   * @returns {function} Unsubscribe function
   */
  subscribe(pattern, handler) {
    if (!this.handlers.has(pattern)) {
      this.handlers.set(pattern, []);
    }
    this.handlers.get(pattern).push(handler);

    // Send subscription to server if connected
    if (this.connected && !this.subscriptions.includes(pattern)) {
      this.subscriptions.push(pattern);
      this._sendSubscriptions();
    }

    // Return unsubscribe function
    return () => {
      const handlers = this.handlers.get(pattern);
      if (handlers) {
        const idx = handlers.indexOf(handler);
        if (idx >= 0) handlers.splice(idx, 1);
        if (handlers.length === 0) this.handlers.delete(pattern);
      }
    };
  }

  /**
   * Subscribe to multiple patterns at once.
   */
  subscribeMany(patterns, handler) {
    const unsubs = patterns.map(p => this.subscribe(p, handler));
    return () => unsubs.forEach(fn => fn());
  }

  /**
   * Get the current snapshot.
   */
  getSnapshot() {
    return this.currentSnapshot;
  }

  /**
   * Request a fresh snapshot from the server.
   */
  requestSnapshot() {
    if (this.connected) {
      this._send({ type: 'get_snapshot' });
    }
  }

  /**
   * Send a custom message to the server.
   */
  send(type, data = {}) {
    this._send({ type, ...data });
  }

  // ─────────────────────────────────────────────────────────────────
  // Private methods
  // ─────────────────────────────────────────────────────────────────

  _setupHandlers() {
    this.ws.onopen = () => {
      this.connected = true;
      this.reconnectAttempts = 0;
      console.log('[EventBus] Connected');

      // Send subscriptions and request initial state
      this._sendSubscriptions();
      this.onConnect();
    };

    this.ws.onclose = (event) => {
      this.connected = false;
      console.log('[EventBus] Disconnected', event.code, event.reason);
      this.onDisconnect(event);

      if (this.autoReconnect) {
        this._scheduleReconnect();
      }
    };

    this.ws.onerror = (error) => {
      console.error('[EventBus] WebSocket error:', error);
      this.onError(error);
    };

    this.ws.onmessage = (message) => {
      try {
        const data = JSON.parse(message.data);
        this._handleMessage(data);
      } catch (err) {
        console.error('[EventBus] Failed to parse message:', err);
      }
    };
  }

  _handleMessage(data) {
    // Handle different message types
    if (data.ns) {
      // This is an Event
      const event = new Event(data);
      this.lastSeq = Math.max(this.lastSeq, event.seq);
      this._dispatch(event);
    } else if (data.type === 'snapshot' || data.id) {
      // This is a Snapshot
      this.currentSnapshot = new Snapshot(data);
      this.lastSeq = this.currentSnapshot.lastSeq;
      this.onSnapshot(this.currentSnapshot);
    } else if (data.type === 'client_versions') {
      // Legacy compatibility: convert to event
      const event = new Event({
        ns: NS.VERSIONS_SYNC,
        ts: Date.now(),
        seq: ++this.lastSeq,
        data: {
          versions: data.versions,
          server_version: data.server_version
        }
      });
      this._dispatch(event);
    } else if (data.type === 'ping_update') {
      // Legacy compatibility: convert to event
      const event = new Event({
        ns: NS.HEALTH_PING,
        ts: data.timestamp || Date.now(),
        seq: ++this.lastSeq,
        data: {
          target: data.target,
          latency: data.latency
        }
      });
      this._dispatch(event);
    }
    // Add more legacy conversions as needed
  }

  _dispatch(event) {
    for (const [pattern, handlers] of this.handlers) {
      if (event.matches(pattern)) {
        handlers.forEach(h => {
          try {
            h(event);
          } catch (err) {
            console.error('[EventBus] Handler error:', err);
          }
        });
      }
    }
  }

  _send(data) {
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(data));
    }
  }

  _sendSubscriptions() {
    this._send({
      type: 'subscribe',
      namespaces: this.subscriptions,
      last_seq: this.lastSeq
    });
  }

  _scheduleReconnect() {
    this.reconnectAttempts++;
    const delay = Math.min(
      this.reconnectDelay * Math.pow(2, this.reconnectAttempts - 1),
      this.maxReconnectDelay
    );
    console.log(`[EventBus] Reconnecting in ${delay}ms (attempt ${this.reconnectAttempts})`);
    setTimeout(() => this.connect(), delay);
  }
}

/**
 * Check if a namespace matches a pattern.
 * Supports wildcards: "versions.*" matches "versions.client_updated"
 */
function matchPattern(pattern, namespace) {
  // Exact match
  if (pattern === namespace) return true;

  // Wildcard match
  if (pattern.endsWith('.*')) {
    const prefix = pattern.slice(0, -2);
    return namespace.startsWith(prefix + '.');
  }

  // Double wildcard matches everything
  if (pattern === '*' || pattern === '**') return true;

  return false;
}

/**
 * Create a new EventBus client.
 */
function createEventBus(options = {}) {
  return new EventBusClient(options);
}

// Export for both Node.js and browser
if (typeof module !== 'undefined' && module.exports) {
  module.exports = {
    NS, SUB, Event, Snapshot, EventBusClient, createEventBus, matchPattern
  };
} else if (typeof window !== 'undefined') {
  window.EventBus = {
    NS, SUB, Event, Snapshot, EventBusClient, createEventBus, matchPattern
  };
}

/*
 * Example Usage:
 *
 * // Create and connect
 * const bus = createEventBus({
 *   url: 'ws://10.8.0.1:9000/ws?vpn_ip=10.8.0.16',
 *   subscriptions: ['versions.*', 'health.*'],
 *   onSnapshot: (snapshot) => {
 *     console.log('Initial state:', snapshot.state);
 *     renderVersions(snapshot.get('versions'));
 *   }
 * });
 *
 * // Subscribe to specific events
 * bus.subscribe('versions.client', (event) => {
 *   console.log('Client version updated:', event.data);
 * });
 *
 * // Subscribe to all health events
 * bus.subscribe('health.*', (event) => {
 *   console.log('Health event:', event.ns, event.data);
 * });
 *
 * // Connect
 * bus.connect();
 *
 * // Later, disconnect
 * bus.disconnect();
 */
