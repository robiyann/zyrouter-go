const fs = require(fs);
const path = require(path);

const projectRoot = __dirname;

function loadEnvFile(filePath) {
  if (!fs.existsSync(filePath)) return;
  try {
    const content = fs.readFileSync(filePath, utf-8);
    for (const line of content.split(
)) {
      const trimmed = line.trim();
      if (!trimmed || trimmed.startsWith(#)) continue;
      const eqIdx = trimmed.indexOf(=);
      if (eqIdx === -1) continue;
      const key = trimmed.slice(0, eqIdx).trim();
      let val = trimmed.slice(eqIdx + 1).trim();
      if ((val.startsWith(') && val.endsWith(')) || (val.startsWith(") && val.endsWith("))) {
        val = val.slice(1, -1);
      }
      if (!process.env[key]) {
        process.env[key] = val;
      }
    }
  } catch (e) {
    // Silent fail on unreadable env
  }
}

// Load environment from root and backend .env files securely
loadEnvFile(path.join(projectRoot, .env));
loadEnvFile(path.join(projectRoot, backend, .env));

module.exports = {
  apps: [
    {
      name: zyrouter,
      cwd: projectRoot,
      script: process.platform === win32 ? ./backend/zyrouter.exe : ./backend/zyrouter,
      interpreter: none,
      exec_mode: fork,
      instances: 1,
      autorestart: true,
      watch: false,
      max_memory_restart: 512M,
      env: {
        NODE_ENV: production,
        LOG_LEVEL: info,
        HOST: 127.0.0.1,
        PORT: 20128,
        FRONTEND_DIR: path.join(projectRoot, frontend),
        DB_PATH: process.env.DB_PATH || /root/.9router/db/data.sqlite,
        CF_EDGE_SHARED_SECRET: process.env.CF_EDGE_SHARED_SECRET || ,
        TELEGRAM_BOT_TOKEN: process.env.TELEGRAM_BOT_TOKEN || ,
        TELEGRAM_BOT_USERNAME: process.env.TELEGRAM_BOT_USERNAME || ,
        TELEGRAM_POLLING_ENABLED: process.env.TELEGRAM_POLLING_ENABLED || true
      }
    },
    {
      name: antigravity-quota-bot,
      cwd: path.join(projectRoot, backend),
      script: process.platform === win32 ? ./antigravity-quota-bot.exe : ./antigravity-quota-bot,
      interpreter: none,
      exec_mode: fork,
      instances: 1,
      autorestart: true,
      watch: false,
      max_memory_restart: 256M,
      env: {
        NODE_ENV: production,
        LOG_LEVEL: info,
        DB_PATH: process.env.DB_PATH || /root/.9router/db/data.sqlite,
        TELEGRAM_QUOTA_BOT_TOKEN: process.env.TELEGRAM_QUOTA_BOT_TOKEN || ,
        TELEGRAM_QUOTA_ALLOWED_USER_IDS: process.env.TELEGRAM_QUOTA_ALLOWED_USER_IDS || 6276972957
      }
    }
  ]
};
