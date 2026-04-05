import { config as loadDotenvConfig } from "dotenv";

import type { EnvMap } from "./config.js";

export function loadStartupEnv(env: EnvMap = process.env): EnvMap {
  const originalKeys = new Set(Object.keys(env).filter((key) => env[key] !== undefined));

  loadDotenvConfig({ processEnv: env, quiet: true });

  const localEnv: EnvMap = {};
  loadDotenvConfig({ path: ".env.local", processEnv: localEnv, quiet: true });

  for (const [key, value] of Object.entries(localEnv)) {
    if (!originalKeys.has(key) && value !== undefined) {
      env[key] = value;
    }
  }

  return env;
}
