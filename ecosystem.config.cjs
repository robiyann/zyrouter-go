module.exports = {
  apps: [
    {
      name: 'zyrouter',
      cwd: __dirname,
      script: './backend/zyrouter',
      interpreter: 'none',
      exec_mode: 'fork',
      instances: 1,
      autorestart: true,
      watch: false,
      max_memory_restart: '512M',
      env: {
        PORT: '20128',
        FRONTEND_DIR: './frontend',
        DATA_DIR: './data'
      }
    }
  ]
};
