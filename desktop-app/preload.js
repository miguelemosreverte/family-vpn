const { contextBridge, ipcRenderer } = require('electron');

// Expose protected methods that allow the renderer process to use
// the ipcRenderer without exposing the entire object
contextBridge.exposeInMainWorld('vpnAPI', {
  getPeers: () => ipcRenderer.invoke('get-vpn-peers'),
  getDiskUsage: (peer) => ipcRenderer.invoke('get-disk-usage', peer),
  getVolumes: (peer) => ipcRenderer.invoke('get-volumes', peer)
});
