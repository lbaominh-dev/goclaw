#!/usr/bin/env node

import process from "node:process";

const args = process.argv.slice(2);
const [subcommand, ...runArgs] = args;

let prompt = "";
for (let i = 0; i < runArgs.length; i += 1) {
  const arg = runArgs[i] ?? "";
  if (arg === "--session") {
    i += 1;
    continue;
  }
  if (arg.startsWith("-")) {
    continue;
  }
  prompt = arg;
}

if (subcommand !== "run") {
  process.stderr.write(`unexpected-subcommand:${subcommand ?? ""}\n`);
  process.exit(2);
}

process.stdout.write(`prompt:${prompt}\n`);
process.stderr.write(`workspace:${process.cwd()}\n`);

if (prompt === "run") {
  const timer = setInterval(() => {
    process.stdout.write("still-running\n");
  }, 50);
  process.on("SIGTERM", () => {
    clearInterval(timer);
    process.exit(143);
  });
} else if (prompt === "fail") {
  process.exit(2);
} else {
  process.exit(0);
}
