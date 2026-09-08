const path = require('path');
const projectRoot = __dirname;

module.exports = {
  apps: [
    {
      name: 'zyrouter',
      cwd: projectRoot,
      script: process.platform === 'win32' ? './backend/zyrouter.exe' : './backend/zyrouter',
      interpreter: 'none',
      exec_mode: 'fork',
      instances: 1,
      autorestart: true,
      watch: false,
      max_memory_restart: '512M',
      env: {
        HOST: '127.0.0.1',
        PORT: '20128',
        FRONTEND_DIR: path.join(projectRoot, 'frontend')
      }
    }
  ]
};
