const { app, BrowserWindow, ipcMain } = require('electron');
const path = require('path');
const http = require('http');
const { exec } = require('child_process');
const fs = require('fs');

let mainWindow;
let devMode = process.argv.includes('--dev');

// Auto-updater using git (same as menu bar app)
function checkForUpdates() {
  const repoDir = path.join(__dirname, '..');

  exec('git -C ' + repoDir + ' fetch origin main', (err) => {
    if (err) return;

    exec('git -C ' + repoDir + ' rev-parse HEAD', (err, localHash) => {
      if (err) return;

      exec('git -C ' + repoDir + ' rev-parse origin/main', (err, remoteHash) => {
        if (err) return;

        if (localHash.trim() !== remoteHash.trim()) {
          console.log('🔄 Update available! Pulling changes...');
          performUpdate();
        }
      });
    });
  });
}

function performUpdate() {
  const repoDir = path.join(__dirname, '..');

  exec('git -C ' + repoDir + ' pull origin main', (err) => {
    if (err) {
      console.error('Failed to pull updates:', err);
      return;
    }

    exec('npm install', { cwd: __dirname }, (err) => {
      if (err) {
        console.error('Failed to install dependencies:', err);
        return;
      }

      console.log('✅ Update complete! Restarting...');
      app.relaunch();
      app.exit(0);
    });
  });
}

function createWindow() {
  mainWindow = new BrowserWindow({
    width: 1400,
    height: 900,
    title: 'Family VPN Dashboard',
    icon: path.join(__dirname, 'assets', 'icon.icns'),
    webPreferences: {
      preload: path.join(__dirname, 'preload.js'),
      contextIsolation: true,
      nodeIntegration: false
    },
    backgroundColor: '#ffffff',
    titleBarStyle: 'hiddenInset' // macOS native look
  });

  mainWindow.loadFile('renderer/index.html');

  if (devMode) {
    mainWindow.webContents.openDevTools();
  }

  mainWindow.on('closed', () => {
    mainWindow = null;
  });
}

// IPC: Get VPN peers from client IPC server (port 8889)
ipcMain.handle('get-vpn-peers', async () => {
  const homeDir = process.env.HOME || process.env.USERPROFILE;
  const peerFile = path.join(homeDir, '.family-vpn-peers.json');

  try {
    const data = fs.readFileSync(peerFile, 'utf8');
    return JSON.parse(data);
  } catch (err) {
    console.error('Failed to read peer list:', err);
    return [];
  }
});

// Helper: Check if peer is local machine
function isLocalPeer(peer) {
  const os = require('os');
  const hostname = os.hostname();
  // Check if peer hostname matches current machine
  return peer.hostname === hostname || peer.hostname.startsWith(hostname.split('.')[0]);
}

// IPC: Get disk usage from a peer via SSH (all physical volumes)
ipcMain.handle('get-disk-usage', async (event, peer) => {
  return new Promise((resolve) => {
    let cmd;

    // Get all volumes, filter out virtual/network ones
    if (isLocalPeer(peer)) {
      cmd = 'df -h | grep -E "^/dev/disk" | grep -v "com.apple"';
    } else {
      const username = getUsernameForPeer(peer);
      cmd = `ssh -o ConnectTimeout=5 -o StrictHostKeyChecking=no ${username}@${peer.vpn_address} "df -h | grep -E '^/dev/disk' | grep -v 'com.apple'"`;
    }

    exec(cmd, (err, stdout, stderr) => {
      if (err) {
        resolve({ error: err.message, peer: peer.hostname });
        return;
      }

      // Parse all physical volumes
      const lines = stdout.trim().split('\n');
      const volumes = lines.map(line => {
        const parts = line.trim().split(/\s+/);
        return {
          filesystem: parts[0],
          size: parts[1],
          used: parts[2],
          available: parts[3],
          usePercent: parts[4],
          mountPoint: parts[8] || parts[5]
        };
      });

      // Calculate totals across all volumes
      let totalSizeGB = 0;
      let totalUsedGB = 0;

      volumes.forEach(vol => {
        const sizeGB = parseSize(vol.size);
        const usedGB = parseSize(vol.used);
        totalSizeGB += sizeGB;
        totalUsedGB += usedGB;
      });

      const totalPercent = totalSizeGB > 0 ? Math.round((totalUsedGB / totalSizeGB) * 100) : 0;

      resolve({
        peer: peer.hostname,
        vpn_address: peer.vpn_address,
        totalSize: `${Math.round(totalSizeGB)}Gi`,
        totalUsed: `${Math.round(totalUsedGB)}Gi`,
        totalAvailable: `${Math.round(totalSizeGB - totalUsedGB)}Gi`,
        usePercent: `${totalPercent}%`,
        volumeCount: volumes.length,
        volumes: volumes
      });
    });
  });
});

// Helper: Parse size string (e.g., "108Gi", "1.5Ti") to GB
function parseSize(sizeStr) {
  const num = parseFloat(sizeStr);
  if (sizeStr.includes('Ti') || sizeStr.includes('T')) {
    return num * 1024;
  } else if (sizeStr.includes('Gi') || sizeStr.includes('G')) {
    return num;
  } else if (sizeStr.includes('Mi') || sizeStr.includes('M')) {
    return num / 1024;
  }
  return num;
}

// IPC: Get all volumes from a peer
ipcMain.handle('get-volumes', async (event, peer) => {
  return new Promise((resolve) => {
    let cmd;

    // Use local command if this is the current machine
    if (isLocalPeer(peer)) {
      cmd = 'df -h';
    } else {
      const username = getUsernameForPeer(peer);
      cmd = `ssh -o ConnectTimeout=5 -o StrictHostKeyChecking=no ${username}@${peer.vpn_address} "df -h"`;
    }

    exec(cmd, (err, stdout, stderr) => {
      if (err) {
        resolve({ error: err.message, peer: peer.hostname });
        return;
      }

      // Parse all volumes
      const lines = stdout.trim().split('\n').slice(1); // Skip header
      const volumes = lines.map(line => {
        const parts = line.trim().split(/\s+/);
        return {
          filesystem: parts[0],
          size: parts[1],
          used: parts[2],
          available: parts[3],
          usePercent: parts[4],
          mountPoint: parts[8] || parts[5]
        };
      });

      resolve({
        peer: peer.hostname,
        vpn_address: peer.vpn_address,
        volumes: volumes
      });
    });
  });
});

function getUsernameForPeer(peer) {
  // Extract username from hostname or use default
  if (peer.hostname.includes('miguel')) {
    return 'miguel_lemos';
  }
  return 'miguel_lemos'; // Default
}

app.whenReady().then(() => {
  createWindow();

  // Check for updates every 5 minutes
  if (!devMode) {
    setInterval(checkForUpdates, 5 * 60 * 1000);
  }
});

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') {
    app.quit();
  }
});

app.on('activate', () => {
  if (mainWindow === null) {
    createWindow();
  }
});
