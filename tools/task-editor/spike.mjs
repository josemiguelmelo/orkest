// Isolated Docker/browser feasibility harness. No production imports or credentials.
import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { randomBytes } from "node:crypto";
import {
  mkdirSync,
  mkdtempSync,
  writeFileSync,
  readFileSync,
  chmodSync,
} from "node:fs";
import http from "node:http";
import { createRequire } from "node:module";
import { resolve } from "node:path";
import { setTimeout as delay } from "node:timers/promises";

const require = createRequire(
  new URL("../../web/package.json", import.meta.url),
);
const { chromium, expect } = require("@playwright/test");
const root = resolve(import.meta.dirname, "../..");
mkdirSync(resolve(root, "tmp/task-editor"), { recursive: true });
const artifacts = mkdtempSync(resolve(root, "tmp/task-editor/run-"));
const image =
  process.env.TASK_EDITOR_IMAGE || "orkest/task-editor-spike:4.145.2";
const docker = (...args) =>
  execFileSync(process.env.DOCKER || "docker", args, {
    encoding: "utf8",
    timeout: 120000,
    stdio: "pipe",
  });
const secret = () => randomBytes(32).toString("hex");
const uid = process.getuid();
const gid = process.getgid();
assert(uid > 0, "Run as the non-root owner of the test checkouts");
const mounts = [];
for (const name of ["alpha", "beta"]) {
  const path = resolve(artifacts, name);
  mkdirSync(resolve(path, ".vscode"), { recursive: true });
  writeFileSync(resolve(path, "README.md"), `${name} existing checkout\n`);
  writeFileSync(
    resolve(path, ".vscode/tasks.json"),
    JSON.stringify({
      version: "2.0.0",
      tasks: [
        {
          label: "automatic-spike",
          type: "shell",
          command: "touch /workspace/alpha/automatic-task-ran",
          runOptions: { runOn: "folderOpen" },
        },
      ],
    }),
  );
  if (name === "alpha") {
    const fixture = resolve(path, ".vscode/vsix");
    mkdirSync(resolve(fixture, "extension"), { recursive: true });
    writeFileSync(
      resolve(fixture, "extension/package.json"),
      JSON.stringify({
        name: "offline-probe",
        publisher: "orkest-spike",
        version: "0.0.1",
        engines: { vscode: "^1.90.0" },
        contributes: {
          configuration: {
            properties: {
              "spike.offline": { type: "boolean", default: false },
            },
          },
        },
      }),
    );
    writeFileSync(
      resolve(fixture, "extension.vsixmanifest"),
      '<PackageManifest Version="2.0.0" xmlns="http://schemas.microsoft.com/developer/vsx-schema/2011"><Metadata><Identity Language="en-US" Id="offline-probe" Version="0.0.1" Publisher="orkest-spike"/><DisplayName>Offline Probe</DisplayName><Description>Isolated extension install probe</Description></Metadata><Installation><InstallationTarget Id="Microsoft.VisualStudio.Code"/></Installation><Dependencies/><Assets><Asset Type="Microsoft.VisualStudio.Code.Manifest" Path="extension/package.json" Addressable="true"/></Assets></PackageManifest>',
    );
    writeFileSync(
      resolve(fixture, "[Content_Types].xml"),
      '<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="json" ContentType="application/json"/></Types>',
    );
  }
  for (const args of [
    ["init", "-b", "task/editor-spike"],
    ["add", "."],
    [
      "-c",
      "user.name=Editor Spike",
      "-c",
      "user.email=spike@example.invalid",
      "-c",
      "commit.gpgsign=false",
      "commit",
      "-m",
      "Existing fixture",
    ],
  ]) {
    execFileSync("git", args, { cwd: path, stdio: "pipe" });
  }
  if (name === "alpha")
    execFileSync(
      "git",
      [
        "archive",
        "--format=zip",
        "-o",
        resolve(path, "probe.vsix"),
        "HEAD:.vscode/vsix",
      ],
      { cwd: path },
    );
  chmodSync(path, 0o777); // Disposable fixtures only: some VM mounts map bind roots to root.
  mounts.push({ path, name });
}
writeFileSync(
  resolve(artifacts, "agent-state-sentinel"),
  "must never be mounted",
);
writeFileSync(
  resolve(artifacts, "credential-sentinel"),
  "synthetic; must never be mounted",
);
const workspace = resolve(artifacts, "task.code-workspace");
writeFileSync(
  workspace,
  JSON.stringify({
    folders: mounts.map(({ name }) => ({ name, path: `/workspace/${name}` })),
  }),
);
let instance;
let target;
let generation = 0;
let currentMode;
const sockets = new Set();
const grants = new Map();
const sessions = new Map();
const platformSession = secret();
let platformOrigin;
let editorOrigin;
let browser;
let gateway;
let platform;
for (const signal of ["SIGINT", "SIGTERM"]) {
  process.once(signal, () => {
    if (instance) {
      try {
        docker("stop", "--time", "2", instance);
        docker("rm", instance);
      } catch (error) {
        console.error("Fixture cleanup failed:", error.message);
      }
    }
    process.exit(130);
  });
}
const evidence = { image, uid, gid, checks: [], observations: {}, artifacts };
const passed = (name) => {
  evidence.checks.push(name);
  console.log(`PASS ${name}`);
};
const cookie = (req, name) =>
  req.headers.cookie
    ?.split(";")
    .map((s) => s.trim())
    .find((s) => s.startsWith(`${name}=`))
    ?.slice(name.length + 1);
const reply = (res, code, body = "") => {
  res.writeHead(code, { "Cache-Control": "no-store" });
  res.end(body);
};
const listen = (server) =>
  new Promise((done) => server.listen(0, "127.0.0.1", done));
const revoke = () => {
  target = undefined;
  grants.clear();
  sessions.clear();
  for (const socket of sockets) socket.destroy();
  sockets.clear();
};
async function stop() {
  revoke();
  if (!instance) return;
  // Concrete container ID, never a name that could select a replacement.
  const old = instance;
  docker("stop", "--time", "2", old);
  assert.equal(JSON.parse(docker("inspect", old))[0].State.Running, false);
  docker("rm", old);
  instance = undefined;
}
async function start(mode) {
  await stop();
  generation++;
  currentMode = mode;
  const args = [
    "run",
    "-d",
    "--init",
    "--read-only",
    "--cap-drop=ALL",
    "--security-opt=no-new-privileges",
    "--pids-limit=256",
    "--memory=1536m",
    "--cpus=2",
    "--label",
    "dev.orkest.task-editor=feasibility",
    "--user",
    `${uid}:${gid}`,
    "-e",
    `EDITOR_MODE=${mode}`,
    "--tmpfs",
    `/home/editor:uid=${uid},gid=${gid},mode=0700`,
    "--tmpfs",
    "/tmp:mode=1777",
    "-p",
    "127.0.0.1::8080",
  ];
  for (const { path, name } of mounts)
    args.push(
      "--mount",
      `type=bind,src=${path},dst=/workspace/${name}${mode === "ro" ? ",readonly" : ""}`,
    );
  args.push(
    "--mount",
    `type=bind,src=${workspace},dst=/workspace/task.code-workspace,readonly`,
    mode === "ro" ? `${image}-ro` : image,
  );
  instance = docker(...args).trim();
  const inspection = JSON.parse(docker("inspect", instance))[0];
  assert.equal(inspection.HostConfig.Privileged, false);
  assert.equal(inspection.Mounts.length, 3);
  assert.deepEqual(Object.keys(inspection.HostConfig.Tmpfs).sort(), [
    "/home/editor",
    "/tmp",
  ]);
  assert.deepEqual(
    inspection.Mounts.filter((m) => m.Type === "bind")
      .map((m) => m.Source)
      .sort(),
    [...mounts.map((m) => m.path), workspace].sort(),
  );
  const port = inspection.NetworkSettings.Ports["8080/tcp"][0].HostPort;
  const upstream = `http://127.0.0.1:${port}`;
  for (let attempt = 0; ; attempt++) {
    try {
      if ((await fetch(`${upstream}/healthz`)).ok) break;
    } catch {
      /* starting */
    }
    if (attempt === 100)
      throw Error(`Editor failed readiness: ${docker("logs", instance)}`);
    await delay(200);
  }
  target = upstream;
  return instance;
}
function authorized(req) {
  const session = sessions.get(cookie(req, "editor_session"));
  return (
    target && session?.generation === generation && session.expires > Date.now()
  );
}
function upstreamHeaders(req) {
  // Allow only editor transport headers; never forward platform cookies/authorization.
  const headers = {};
  for (const name of [
    "accept",
    "accept-encoding",
    "content-type",
    "content-length",
    "user-agent",
    "connection",
    "upgrade",
    "sec-websocket-key",
    "sec-websocket-version",
    "sec-websocket-protocol",
  ]) {
    if (req.headers[name]) headers[name] = req.headers[name];
  }
  headers.host = new URL(target).host;
  if (req.headers.origin) headers.origin = target;
  return headers;
}
async function command(page, name) {
  await page.keyboard.press("F1");
  await page.locator(".quick-input-widget input").first().fill(`>${name}`);
  await expect(
    page.locator(".quick-input-list .monaco-list-row").first(),
  ).toBeVisible();
  await page.keyboard.press("Enter");
  if (name === "Terminal: Create New Terminal") {
    await expect(page.getByRole("option", { name: /^alpha,/ })).toBeVisible();
    await page.keyboard.press("Enter");
  }
  await expect(page.locator(".quick-input-widget")).toBeHidden();
  await delay(400);
}
async function openFile(page, path) {
  await page.keyboard.press("ControlOrMeta+p");
  await page.locator(".quick-input-widget input").first().fill(path);
  await expect(
    page.locator(".quick-input-list .monaco-list-row").first(),
  ).toBeVisible();
  await page.keyboard.press("Enter");
  await expect(page.locator(".quick-input-widget")).toBeHidden();
  await expect(
    page.locator(".editor-group-container .monaco-editor textarea").first(),
  ).toBeAttached();
  await page
    .locator(".editor-group-container .monaco-editor .view-lines")
    .first()
    .click({ position: { x: 80, y: 8 } });
  await delay(500);
}
async function ready(page) {
  await expect(page.locator(".monaco-workbench")).toBeVisible({
    timeout: 60000,
  });
  await expect(page.getByText("alpha", { exact: true }).first()).toBeVisible({
    timeout: 30000,
  });
  await expect(page.getByText("beta", { exact: true }).first()).toBeVisible({
    timeout: 30000,
  });
}
async function launch(page) {
  await page.goto(platformOrigin);
  const popup = page.waitForEvent("popup");
  await page.getByRole("button", { name: "Open VS Code" }).click();
  const editor = await popup;
  await ready(
    editor.frames().find((f) => f.url().includes("/?workspace=")) ||
      editor.frameLocator("#editor"),
  );
  return editor;
}
try {
  evidence.images = {
    rw: docker("image", "inspect", "--format", "{{.Id}}", image).trim(),
    ro: docker("image", "inspect", "--format", "{{.Id}}", `${image}-ro`).trim(),
  };
  evidence.packages = docker(
    "run",
    "--rm",
    "--entrypoint",
    "dpkg-query",
    image,
    "-W",
    "git",
    "bash",
    "curl",
    "ca-certificates",
    "procps",
  ).trim();
  evidence.docker = docker("version", "--format", "{{.Server.Version}}").trim();
  evidence.codeServer = docker(
    "run",
    "--rm",
    "-e",
    "HOME=/tmp",
    "--entrypoint",
    "/opt/code-server/bin/code-server",
    image,
    "--version",
  ).trim();
  gateway = http.createServer((req, res) => {
    const url = new URL(req.url, editorOrigin);
    if (url.pathname === "/exchange") {
      const grant = grants.get(url.searchParams.get("grant"));
      grants.delete(url.searchParams.get("grant"));
      if (
        !target ||
        !grant ||
        grant.generation !== generation ||
        grant.expires < Date.now()
      )
        return reply(res, 403);
      const session = secret();
      sessions.set(session, { generation, expires: Date.now() + 300000 });
      res.writeHead(200, {
        "Content-Type": "text/html",
        "Set-Cookie": `editor_session=${session}; HttpOnly; SameSite=Strict; Path=/`,
        "Cache-Control": "no-store",
        "Referrer-Policy": "no-referrer",
      });
      return res.end(
        '<!doctype html><script>location.replace("/launch")</script>',
      );
    }
    if (!authorized(req)) return reply(res, 403);
    if (url.pathname === "/launch") {
      res.setHeader("Content-Type", "text/html");
      return reply(
        res,
        200,
        `<!doctype html><title>Task editor spike</title><style>html,body{margin:0;height:100%}iframe{width:100%;height:95%;border:0}</style><div id="mode">${currentMode} generation ${generation}</div><iframe id="editor" src="/?workspace=/workspace/task.code-workspace"></iframe><script>
        setInterval(async()=>{const response=await fetch('/generation'); if(!response.ok){document.querySelector('iframe').remove();location.href=${JSON.stringify(platformOrigin + "/reconnect")};}},500);
      </script>`,
      );
    }
    if (url.pathname === "/generation")
      return reply(res, 200, String(generation));
    if (req.headers.origin && req.headers.origin !== editorOrigin)
      return reply(res, 403);
    const upstream = http.request(
      new URL(req.url, target),
      { method: req.method, headers: upstreamHeaders(req) },
      (response) => {
        res.writeHead(response.statusCode, response.headers);
        response.pipe(res);
      },
    );
    upstream.on("error", () => {
      if (!res.headersSent) reply(res, 502);
      else res.destroy();
    });
    req.pipe(upstream);
  });
  gateway.on("upgrade", (req, socket, head) => {
    if (!authorized(req) || req.headers.origin !== editorOrigin)
      return socket.destroy();
    sockets.add(socket);
    socket.on("close", () => sockets.delete(socket));
    const upstream = http.request(new URL(req.url, target), {
      headers: upstreamHeaders(req),
    });
    upstream.on("upgrade", (res, remote, upstreamHead) => {
      sockets.add(remote);
      remote.on("close", () => sockets.delete(remote));
      socket.write(
        `HTTP/1.1 101 Switching Protocols\r\n${Object.entries(res.headers)
          .map(([k, v]) => `${k}: ${v}`)
          .join("\r\n")}\r\n\r\n`,
      );
      if (upstreamHead.length) socket.write(upstreamHead);
      if (head.length) remote.write(head);
      remote.pipe(socket).pipe(remote);
      socket.on("error", () => remote.destroy());
      remote.on("error", () => socket.destroy());
      socket.on("close", () => remote.destroy());
      remote.on("close", () => socket.destroy());
    });
    upstream.on("error", () => socket.destroy());
    upstream.end();
  });
  platform = http.createServer((req, res) => {
    if (cookie(req, "platform_session") !== platformSession)
      return reply(res, 403);
    if (req.url === "/grant") {
      if (req.method !== "POST" || req.headers.origin !== platformOrigin)
        return reply(res, 403);
      if (!target) return reply(res, 503);
      const grant = secret();
      grants.set(grant, { generation, expires: Date.now() + 10000 });
      return reply(res, 200, `${editorOrigin}/exchange?grant=${grant}`);
    }
    res.setHeader("Content-Type", "text/html");
    reply(
      res,
      200,
      `<!doctype html><title>Spike platform</title><button>Open VS Code</button><script>
      async function connect(tab){for(;;){const r=await fetch('/grant',{method:'POST'});if(r.ok){tab.location=await r.text();return;}await new Promise(r=>setTimeout(r,500));}}
      document.querySelector('button').onclick=()=>connect(window.open('about:blank'));
      if(location.pathname==='/reconnect')connect(window);
    </script>`,
    );
  });
  await listen(gateway);
  await listen(platform);
  editorOrigin = `http://localhost:${gateway.address().port}`;
  platformOrigin = `http://127.0.0.1:${platform.address().port}`;
  assert.equal((await fetch(editorOrigin)).status, 403);
  assert.equal((await fetch(platformOrigin)).status, 403);
  passed("unauthenticated platform/editor HTTP denied");
  await start("rw");
  const grantResponse = await fetch(`${platformOrigin}/grant`, {
    method: "POST",
    headers: {
      Cookie: `platform_session=${platformSession}`,
      Origin: platformOrigin,
    },
  });
  assert.equal(grantResponse.status, 200);
  const grantURL = await grantResponse.text();
  assert.equal((await fetch(grantURL)).status, 200);
  assert.equal((await fetch(grantURL)).status, 403);
  assert.equal(
    (
      await fetch(`${platformOrigin}/grant`, {
        method: "POST",
        headers: {
          Cookie: `platform_session=${platformSession}`,
          Origin: editorOrigin,
        },
      })
    ).status,
    403,
  );
  passed(
    "launch grant is single-use and cross-origin lifecycle request denied",
  );
  browser = await chromium.launch({
    executablePath: process.env.TASK_EDITOR_BROWSER || undefined,
  });
  evidence.browser = browser.version();
  const context = await browser.newContext({
    viewport: { width: 1440, height: 1000 },
  });
  await context.addCookies([
    {
      name: "platform_session",
      value: platformSession,
      url: platformOrigin,
      httpOnly: true,
      sameSite: "Lax",
    },
  ]);
  const page = await context.newPage();
  const editor = await launch(page);
  const frame = editor.frameLocator("#editor");
  // Commands and keyboard operate on the iframe via the page after clicking it.
  await frame
    .locator(".monaco-workbench")
    .click({ position: { x: 600, y: 300 } });
  const ui = {
    keyboard: editor.keyboard,
    getByRole: (...args) => frame.getByRole(...args),
    locator: (...args) => frame.locator(...args),
  };
  passed(
    "separate-origin launch, static assets, WebSockets and multi-root explorer",
  );
  await openFile(ui, "/workspace/alpha/README.md");
  await editor.keyboard.press("ControlOrMeta+a");
  await editor.keyboard.type("saved in browser\n");
  await editor.keyboard.press("ControlOrMeta+s");
  await expect
    .poll(() => readFileSync(resolve(artifacts, "alpha/README.md"), "utf8"))
    .toBe("saved in browser\n");
  passed("browser edits and saves existing checkout");
  docker(
    "exec",
    instance,
    "/opt/code-server/bin/code-server",
    "--install-extension",
    "/workspace/alpha/probe.vsix",
  );
  assert.match(
    docker(
      "exec",
      instance,
      "/opt/code-server/bin/code-server",
      "--list-extensions",
    ),
    /orkest-spike.offline-probe/,
  );
  passed("valid offline VSIX installs in writable mode");
  await command(ui, "Terminal: Create New Terminal");
  const terminal = frame.locator(".terminal-wrapper textarea").first();
  await expect(terminal).toBeAttached({ timeout: 20000 });
  await terminal.focus();
  await editor.keyboard.type(
    "git -C /workspace/alpha status --short > /workspace/alpha/git-status; echo terminal-ok > /workspace/beta/untracked.txt; bash -c 'while true; do date +%s%N >> /workspace/alpha/heartbeat; sleep 0.2; done' & wait",
    { delay: 1 },
  );
  await editor.keyboard.press("Enter");
  await expect
    .poll(() => {
      try {
        return readFileSync(resolve(artifacts, "beta/untracked.txt"), "utf8");
      } catch {
        return "";
      }
    })
    .toBe("terminal-ok\n");
  await expect
    .poll(() => {
      try {
        return readFileSync(resolve(artifacts, "alpha/heartbeat"), "utf8")
          .length;
      } catch {
        return 0;
      }
    })
    .toBeGreaterThan(20);
  assert.match(
    readFileSync(resolve(artifacts, "alpha/git-status"), "utf8"),
    /M README.md/,
  );
  passed("integrated terminal executes Git and writes across repository roots");
  await openFile(ui, "/workspace/alpha/README.md");
  await editor.keyboard.press("ControlOrMeta+End");
  await editor.keyboard.type("UNSAVED-SPIKE");
  await editor.screenshot({ path: resolve(artifacts, "rw.png") });
  const oldCookie = (await context.cookies(editorOrigin)).find(
    (c) => c.name === "editor_session",
  ).value;
  const oldID = instance;
  evidence.processesBeforeStop = docker("top", oldID, "-eo", "pid,ppid,args");
  await start("ro");
  const stoppedSize = readFileSync(
    resolve(artifacts, "alpha/heartbeat"),
    "utf8",
  ).length;
  await delay(1000);
  assert.equal(
    readFileSync(resolve(artifacts, "alpha/heartbeat"), "utf8").length,
    stoppedSize,
  );
  assert.equal(
    (
      await fetch(`${editorOrigin}/generation`, {
        headers: { Cookie: `editor_session=${oldCookie}` },
      })
    ).status,
    403,
  );
  passed(
    "recreation confirms container termination, stops terminal descendants and revokes old browser session",
  );
  await expect(editor.locator("#mode")).toHaveText(
    `${currentMode} generation ${generation}`,
    { timeout: 60000 },
  );
  await ready(frame);
  await frame
    .locator(".monaco-workbench")
    .click({ position: { x: 600, y: 300 } });
  await openFile(ui, "/workspace/alpha/README.md");
  const restoredText = await frame
    .locator(".editor-group-container .view-lines")
    .first()
    .innerText();
  evidence.observations.unsavedBuffer = restoredText.includes("UNSAVED-SPIKE")
    ? "Recovered by browser storage after recreation"
    : "Unsaved edit lost after recreation";
  assert.equal(
    readFileSync(resolve(artifacts, "alpha/README.md"), "utf8"),
    "saved in browser\n",
  );
  assert.equal(
    readFileSync(resolve(artifacts, "beta/untracked.txt"), "utf8"),
    "terminal-ok\n",
  );
  for (const { name } of mounts) {
    assert.equal(
      docker(
        "exec",
        instance,
        "git",
        "-C",
        `/workspace/${name}`,
        "branch",
        "--show-current",
      ).trim(),
      "task/editor-spike",
    );
    for (const path of [
      `/workspace/${name}/README.md`,
      `/workspace/${name}/.git/config`,
    ]) {
      assert.throws(
        () =>
          docker(
            "exec",
            instance,
            "bash",
            "-c",
            'echo forbidden >> "$1"',
            "spike",
            path,
          ),
        /Read-only file system/,
      );
    }
  }
  passed(
    "read-only mounts reject file and Git metadata writes; saved/untracked files and branches survive",
  );
  await command(ui, "Terminal: Create New Terminal");
  await delay(1500);
  await expect(
    frame
      .getByText(/Task editor is read-only: terminal and tasks are disabled/)
      .first(),
  ).toBeVisible({ timeout: 15000 });
  evidence.observations.readonlyTerminal = await frame
    .locator(".terminal")
    .allTextContents();
  evidence.processesReadonly = docker("top", instance, "-eo", "pid,ppid,args");
  assert(
    !/^\d+\s+\d+\s+\/(?:usr\/)?bin\/(?:bash|sh)(?:\s|$)/m.test(
      evidence.processesReadonly,
    ),
    "Read-only terminal must not spawn a shell",
  );
  assert.throws(
    () => docker("exec", instance, "touch", "/opt/extensions/forbidden"),
    /Read-only file system/,
  );
  assert.throws(
    () =>
      docker(
        "exec",
        instance,
        "/opt/code-server/bin/code-server",
        "--extensions-dir",
        "/opt/extensions",
        "--install-extension",
        "/workspace/alpha/probe.vsix",
      ),
    /Failed Installing Extensions|Error while installing extensions/i,
  );
  assert.doesNotMatch(
    docker(
      "exec",
      instance,
      "/opt/code-server/bin/code-server",
      "--extensions-dir",
      "/opt/extensions",
      "--list-extensions",
    ),
    /orkest-spike.offline-probe/,
  );
  assert.throws(() =>
    readFileSync(resolve(artifacts, "alpha/automatic-task-ran")),
  );
  await editor.screenshot({ path: resolve(artifacts, "ro.png") });
  passed(
    "read-only terminal denied, extension directory immutable and automatic task did not run",
  );
  await start("rw");
  await expect(editor.locator("#mode")).toHaveText(
    `${currentMode} generation ${generation}`,
    { timeout: 60000 },
  );
  await ready(frame);
  passed(
    "stable launch page reconnects again after read-only to read/write recreation",
  );
  evidence.result = "passed";
} catch (error) {
  evidence.result = "failed";
  evidence.error = error.stack;
  if (browser) {
    for (const [index, page] of browser
      .contexts()
      .flatMap((context) => context.pages())
      .entries()) {
      await page
        .screenshot({ path: resolve(artifacts, `failure-${index}.png`) })
        .catch(() => {});
      writeFileSync(
        resolve(artifacts, `failure-${index}.html`),
        await page.content(),
      );
    }
  }
  if (instance) {
    try {
      writeFileSync(
        resolve(artifacts, "container.log"),
        docker("logs", instance),
      );
    } catch {
      /* retain original failure */
    }
  }
  console.error(error);
  process.exitCode = 1;
} finally {
  await browser?.close();
  await stop();
  gateway?.closeAllConnections();
  platform?.closeAllConnections();
  gateway?.close();
  platform?.close();
  writeFileSync(
    resolve(artifacts, "evidence.json"),
    JSON.stringify(evidence, null, 2),
  );
  console.log(`Evidence: ${artifacts}`);
}
