#!/usr/bin/env node

import process from "node:process";

const [subcommand, prompt = ""] = process.argv.slice(2);

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
