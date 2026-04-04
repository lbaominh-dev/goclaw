import { createServer as createHttpServer, type IncomingMessage } from "node:http";

import WebSocket, { WebSocketServer } from "ws";

import type { WorkerConfig } from "./config.js";
import { JobManager } from "./job-manager.js";
import { createOpencodeRunner } from "./opencode-runner.js";
import { decodeEnvelope, encodeEnvelope, parseCancelPayload, parseDispatchPayload, type WorkerReply } from "./protocol.js";

export function createServer(config: WorkerConfig) {
  const httpServer = createHttpServer();
  const websocketServer = new WebSocketServer({ noServer: true });

  httpServer.on("upgrade", (request, socket, head) => {
    if ((request.url ?? "") !== config.path) {
      socket.write("HTTP/1.1 404 Not Found\r\nConnection: close\r\n\r\n");
      socket.destroy();
      return;
    }
    if (!isAuthorized(request, config)) {
      socket.write("HTTP/1.1 401 Unauthorized\r\nConnection: close\r\n\r\n");
      socket.destroy();
      return;
    }
    websocketServer.handleUpgrade(request, socket, head, (websocket) => {
      websocketServer.emit("connection", websocket, request);
    });
  });

  websocketServer.on("connection", (websocket) => {
    const manager = new JobManager({
      workspaces: config.workspaces,
      runJob: createOpencodeRunner(config),
      send: (message: WorkerReply) => {
        if (websocket.readyState === WebSocket.OPEN) {
          websocket.send(encodeEnvelope(message));
        }
      },
    });

    websocket.on("message", async (data) => {
      try {
        const envelope = decodeEnvelope(String(data));
        switch (envelope.type) {
          case "job.dispatch": {
            const payload = parseDispatchPayload(envelope.payload);
            void manager.dispatch(payload).catch((error: unknown) => {
              websocket.send(
                encodeEnvelope({
                  type: "job.failed",
                  jobId: payload.jobId,
                  error: { message: error instanceof Error ? error.message : "job dispatch failed" },
                }),
              );
            });
            break;
          }
          case "job.cancel": {
            const payload = parseCancelPayload(envelope.payload);
            await manager.cancel(payload);
            break;
          }
          default:
            break;
        }
      } catch (error) {
        websocket.send(
          encodeEnvelope({
            type: "job.failed",
            jobId: "",
            error: { message: error instanceof Error ? error.message : "invalid message" },
          }),
        );
      }
    });

    websocket.once("close", () => {
      void manager.cancelAll();
    });
  });

  return {
    async start(): Promise<void> {
      await new Promise<void>((resolve) => {
        httpServer.listen(config.port, "127.0.0.1", () => resolve());
      });
    },
    async stop(): Promise<void> {
      await new Promise<void>((resolve, reject) => {
        websocketServer.close((error) => {
          if (error) {
            reject(error);
            return;
          }
          httpServer.close((closeError) => {
            if (closeError) {
              reject(closeError);
              return;
            }
            resolve();
          });
        });
      });
    },
    get url(): string {
      const address = httpServer.address();
      if (!address || typeof address === "string") {
        throw new Error("server is not listening");
      }
      return `ws://127.0.0.1:${address.port}${config.path}`;
    },
  };
}

function isAuthorized(request: IncomingMessage, config: WorkerConfig): boolean {
  const actual = request.headers[config.authHeader.toLowerCase()];
  if (Array.isArray(actual)) {
    return actual.includes(config.authToken);
  }
  return actual === config.authToken;
}
