const { app, BrowserWindow, ipcMain } = require('electron');
const path = require('path');
const http = require('http');
const { exec } = require('child_process');
const fs = require('fs');

let mainWindow;
let devMode = process.argv.includes('--dev');

// Single instance lock - only allow one instance of the app
const gotTheLock = app.requestSingleInstanceLock();

if (!gotTheLock) {
  // Another instance is already running, quit this one
  app.quit();
} else {
  // Focus the existing window if user tries to open a second instance
  app.on('second-instance', (event, commandLine, workingDirectory) => {
    if (mainWindow) {
      if (mainWindow.isMinimized()) mainWindow.restore();
      mainWindow.focus();
    }
  });
}

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
      cmd = `ssh -o ConnectTimeout=15 -o StrictHostKeyChecking=no ${username}@${peer.vpn_address} "df -h | grep -E '^/dev/disk' | grep -v 'com.apple'"`;
    }

    exec(cmd, { timeout: 20000 }, (err, stdout, stderr) => {
      if (err) {
        // Provide clearer error messages
        let errorMsg = 'Unable to connect';
        if (err.message.includes('timeout') || err.message.includes('timed out')) {
          errorMsg = 'Connection timeout - peer may be offline';
        } else if (err.message.includes('refused')) {
          errorMsg = 'SSH disabled or refused';
        }
        resolve({ error: errorMsg, peer: peer.hostname, vpn_address: peer.vpn_address });
        return;
      }

      // Parse all physical volumes and filter out system-reserved ones
      const lines = stdout.trim().split('\n');
      const allVolumes = lines.map(line => {
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

      // Filter out system-reserved volumes that users can't access
      const systemPaths = [
        '/System/Volumes/VM',           // Virtual memory/swap
        '/System/Volumes/Preboot',      // Boot configuration
        '/System/Volumes/Update',       // System updates
        '/System/Volumes/xarts',        // System
        '/System/Volumes/iSCPreboot',   // System boot
        '/System/Volumes/Hardware',     // Hardware config
      ];

      const volumes = allVolumes.filter(vol => {
        const mount = vol.mountPoint;
        // Exclude exact matches to system paths
        if (systemPaths.includes(mount)) return false;
        // Exclude anything under Update/mnt
        if (mount.includes('/Update/mnt')) return false;
        return true;
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
      cmd = `ssh -o ConnectTimeout=15 -o StrictHostKeyChecking=no ${username}@${peer.vpn_address} "df -h"`;
    }

    exec(cmd, { timeout: 20000 }, (err, stdout, stderr) => {
      if (err) {
        // Provide clearer error messages
        let errorMsg = 'Unable to connect';
        if (err.message.includes('timeout') || err.message.includes('timed out')) {
          errorMsg = 'Connection timeout - peer may be offline';
        } else if (err.message.includes('refused')) {
          errorMsg = 'SSH disabled or refused';
        }
        resolve({ error: errorMsg, peer: peer.hostname, vpn_address: peer.vpn_address });
        return;
      }

      // Parse all volumes and filter out system-reserved ones
      const lines = stdout.trim().split('\n').slice(1); // Skip header
      const allVolumes = lines.map(line => {
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

      // Filter out system-reserved volumes
      const systemPaths = [
        '/System/Volumes/VM',
        '/System/Volumes/Preboot',
        '/System/Volumes/Update',
        '/System/Volumes/xarts',
        '/System/Volumes/iSCPreboot',
        '/System/Volumes/Hardware',
      ];

      const volumes = allVolumes.filter(vol => {
        const mount = vol.mountPoint;
        if (systemPaths.includes(mount)) return false;
        if (mount.includes('/Update/mnt')) return false;
        return true;
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
  // Extract username from hostname
  const hostname = peer.hostname.toLowerCase();

  if (hostname.includes('anastasiia')) {
    return 'anastasiia';
  }

  // Default to miguel_lemos for all other machines
  return 'miguel_lemos';
}

// IPC: Get feature flags
ipcMain.handle('get-feature-flags', async () => {
  const flagsPath = path.join(__dirname, 'feature-flags.json');

  try {
    const data = fs.readFileSync(flagsPath, 'utf8');
    return JSON.parse(data);
  } catch (err) {
    console.error('Failed to read feature flags:', err);
    return {
      version: '1.0.0',
      features: {
        autoUpdate: { enabled: true, checkIntervalMinutes: 5 }
      }
    };
  }
});

// IPC: Check for UI updates in Git
ipcMain.handle('check-ui-updates', async () => {
  const repoDir = path.join(__dirname, '..');

  return new Promise((resolve) => {
    // Fetch latest from Git
    exec('git -C ' + repoDir + ' fetch origin main', (err) => {
      if (err) {
        console.error('Failed to fetch from Git:', err);
        resolve(false);
        return;
      }

      // Check for changes in desktop-app/renderer directory
      exec(
        'git -C ' + repoDir + ' diff HEAD origin/main -- desktop-app/renderer desktop-app/feature-flags.json',
        (err, stdout) => {
          if (err) {
            resolve(false);
            return;
          }

          const hasUIChanges = stdout.trim().length > 0;
          console.log(`🔍 UI changes detected: ${hasUIChanges}`);
          resolve(hasUIChanges);
        }
      );
    });
  });
});

// IPC: Pull UI updates from Git
ipcMain.handle('pull-ui-updates', async () => {
  const repoDir = path.join(__dirname, '..');

  return new Promise((resolve) => {
    // Pull only UI files
    exec(
      'git -C ' + repoDir + ' pull origin main',
      (err, stdout, stderr) => {
        if (err) {
          console.error('Failed to pull UI updates:', err);
          resolve(false);
          return;
        }

        console.log('✅ UI updates pulled from Git');
        resolve(true);
      }
    );
  });
});

// IPC: Trigger full reinstall (Cmd+Shift+U)
ipcMain.handle('trigger-reinstall', async () => {
  const repoDir = path.join(__dirname, '..');
  const installScript = path.join(repoDir, 'install.sh');

  return new Promise((resolve, reject) => {
    console.log('🔧 Starting full reinstall...');

    // Run install script in background
    const child = exec(
      `bash "${installScript}"`,
      { cwd: repoDir, timeout: 300000 }, // 5 minute timeout
      (err, stdout, stderr) => {
        if (err) {
          console.error('❌ Reinstall failed:', err);
          console.error('stderr:', stderr);
          reject(new Error('Reinstall failed: ' + err.message));
          return;
        }

        console.log('✅ Reinstall complete!');
        console.log('stdout:', stdout);

        // Restart app after successful install
        setTimeout(() => {
          app.relaunch();
          app.exit(0);
        }, 2000);

        resolve(true);
      }
    );

    // Stream output to console
    child.stdout.on('data', (data) => {
      console.log('[install]', data.toString().trim());
    });

    child.stderr.on('data', (data) => {
      console.error('[install-err]', data.toString().trim());
    });
  });
});

// Watch for menu bar app - quit desktop if menu bar quits
function watchMenuBarApp() {
  setInterval(() => {
    exec('pgrep -f family-vpn-menubar', (err, stdout) => {
      if (err || !stdout.trim()) {
        console.log('Menu bar app not running, quitting desktop app...');
        app.quit();
      }
    });
  }, 2000); // Check every 2 seconds
}

app.whenReady().then(() => {
  createWindow();

  // Check for updates every 5 minutes
  if (!devMode) {
    setInterval(checkForUpdates, 5 * 60 * 1000);
  }

  // Watch menu bar app
  watchMenuBarApp();
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
