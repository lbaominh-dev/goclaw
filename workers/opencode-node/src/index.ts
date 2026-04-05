import { loadConfig } from "./config.js";
import { loadStartupEnv } from "./env-loader.js";
import { createServer } from "./server.js";

async function main(): Promise<void> {
  const config = loadConfig(undefined, loadStartupEnv());
  const server = createServer(config);
  await server.start();
}

void main();
