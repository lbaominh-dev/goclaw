import process from "node:process";

const chunks = [];

process.stdin.setEncoding("utf8");
process.stdin.on("data", (chunk) => {
  chunks.push(chunk);
});

process.stdin.on("end", () => {
  const payload = JSON.parse(chunks.join(""));
  process.stdout.write(`job:${payload.jobId}\n`);
  process.stderr.write(`workspace:${process.cwd()}\n`);

  if (payload.job?.holdOpen) {
    const timer = setInterval(() => {
      process.stdout.write("still-running\n");
    }, 50);
    process.on("SIGTERM", () => {
      clearInterval(timer);
      process.exit(143);
    });
    return;
  }

  if (payload.job?.fail) {
    process.exit(2);
    return;
  }

  process.exit(0);
});
