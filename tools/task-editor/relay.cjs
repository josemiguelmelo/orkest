"use strict";
const http = require("node:http");
const crypto = require("node:crypto");
const token = process.env.ORKEST_EDITOR_RELAY_TOKEN;
const target = new URL(process.env.ORKEST_EDITOR_RELAY_UPSTREAM);
if (
  !token ||
  token.length !== 64 ||
  target.protocol !== "http:" ||
  !target.hostname
)
  process.exit(1);
const expected = Buffer.from(token);
function authorized(request) {
  const actual = Buffer.from(request.headers["x-orkest-editor-relay"] || "");
  return (
    actual.length === expected.length &&
    crypto.timingSafeEqual(actual, expected)
  );
}
function headers(request) {
  const result = { ...request.headers, host: target.host };
  delete result["x-orkest-editor-relay"];
  delete result.authorization;
  delete result.cookie;
  delete result["proxy-authorization"];
  if (result.origin) result.origin = target.origin;
  return result;
}
const server = http.createServer((request, response) => {
  if (!authorized(request)) {
    response.writeHead(403).end();
    return;
  }
  const upstream = http.request(
    {
      hostname: target.hostname,
      port: target.port,
      method: request.method,
      path: request.url,
      headers: headers(request),
    },
    (remote) => {
      response.writeHead(remote.statusCode, remote.headers);
      remote.pipe(response);
      remote.on("error", () => response.destroy());
    },
  );
  upstream.on("error", () => {
    if (!response.headersSent) response.writeHead(502);
    response.end();
  });
  response.on("close", () => upstream.destroy());
  request.on("aborted", () => upstream.destroy());
  request.on("error", () => upstream.destroy());
  request.pipe(upstream);
});
server.on("upgrade", (request, socket, head) => {
  if (!authorized(request)) {
    socket.end("HTTP/1.1 403 Forbidden\r\nConnection: close\r\n\r\n");
    return;
  }
  const upstream = http.request({
    hostname: target.hostname,
    port: target.port,
    method: request.method,
    path: request.url,
    headers: headers(request),
  });
  upstream.on("upgrade", (response, remote, upstreamHead) => {
    socket.write(
      `HTTP/1.1 101 Switching Protocols\r\n${Object.entries(response.headers)
        .map(([key, value]) => `${key}: ${value}\r\n`)
        .join("")}\r\n`,
    );
    if (head.length) remote.write(head);
    if (upstreamHead.length) socket.write(upstreamHead);
    socket.on("error", () => remote.destroy());
    remote.on("error", () => socket.destroy());
    socket.on("close", () => remote.destroy());
    remote.on("close", () => socket.destroy());
    socket.pipe(remote).pipe(socket);
  });
  upstream.on("response", (response) => {
    response.resume();
    socket.end("HTTP/1.1 502 Bad Gateway\r\nConnection: close\r\n\r\n");
  });
  upstream.on("error", () => socket.destroy());
  socket.on("close", () => upstream.destroy());
  socket.on("error", () => upstream.destroy());
  upstream.end();
});
server.requestTimeout = 30000;
server.headersTimeout = 10000;
server.listen(8081, "0.0.0.0");
