const { contextBridge, ipcRenderer } = require('electron');
const path = require('path');

// Load EventBus from protocol directory
const { NS, SUB, createEventBus, matchPattern } = require(path.join(__dirname, '..', 'protocol', 'events.js'));

// Expose protected methods that allow the renderer process to use
// the ipcRenderer without exposing the entire object
contextBridge.exposeInMainWorld('vpnAPI', {
  getPeers: () => ipcRenderer.invoke('get-vpn-peers'),
  getDiskUsage: (peer) => ipcRenderer.invoke('get-disk-usage', peer),
  getVolumes: (peer) => ipcRenderer.invoke('get-volumes', peer),

  // Feature flags and updates
  getFeatureFlags: () => ipcRenderer.invoke('get-feature-flags'),
  checkForUIUpdates: () => ipcRenderer.invoke('check-ui-updates'),
  pullUIUpdates: () => ipcRenderer.invoke('pull-ui-updates'),
  triggerReinstall: () => ipcRenderer.invoke('trigger-reinstall'),

  // Server info
  getServerInfo: () => ipcRenderer.invoke('get-server-info')
});

// Expose EventBus for real-time events
contextBridge.exposeInMainWorld('EventBus', {
  NS,
  SUB,
  createEventBus,
  matchPattern
});
