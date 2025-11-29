const { contextBridge, ipcRenderer } = require('electron');

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
  triggerReinstall: () => ipcRenderer.invoke('trigger-reinstall')
});
