"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const bridge = require("../shared.js");

const root = path.resolve(__dirname, "..");
const read = (name) => fs.readFileSync(path.join(root, name), "utf8");

test("manifest is MV3 with minimal permissions and intended fixed origins", () => {
  const manifest = JSON.parse(read("manifest.json"));
  assert.equal(manifest.manifest_version, 3);
  assert.deepEqual(manifest.permissions, ["alarms", "cookies", "scripting", "storage"]);
  assert.deepEqual(manifest.host_permissions, ["https://cursor.com/*"]);
  assert.deepEqual(manifest.optional_host_permissions, ["https://*/*"]);
  assert.equal(manifest.permissions.includes("tabs"), false);
  assert.equal(manifest.permissions.includes("webRequest"), false);
  assert.equal(manifest.permissions.includes("debugger"), false);

  const matches = manifest.content_scripts.flatMap((entry) => entry.matches);
  for (const expected of [
    "https://www.poke2api.com/*",
    "https://www.pokeapi.top/*",
    "http://localhost/*",
    "https://localhost/*",
    "http://127.0.0.1/*",
    "https://127.0.0.1/*"
  ]) assert.ok(matches.includes(expected));
  assert.equal(matches.some((value) => value.includes("cursor.com")), false);
});

test("optional origins require an exact public HTTPS origin", () => {
  assert.equal(bridge.normalizeOptionalOrigin("https://example.com:8443"), "https://example.com:8443");
  assert.equal(bridge.originPattern("https://example.com:8443"), "https://example.com:8443/*");
  for (const invalid of [
    "http://example.com",
    "https://user@example.com",
    "https://example.com/path",
    "https://example.com/?query=1",
    "https://www.poke2api.com"
  ]) assert.throws(() => bridge.normalizeOptionalOrigin(invalid));
});

test("flow envelopes bind protocol version and all random tokens", () => {
  const flow = {
    channel: bridge.CHANNEL,
    v: bridge.VERSION,
    direction: "page-to-extension",
    type: "START",
    flowId: "flow_1234567890123456",
    nonce: "nonce_123456789012345",
    bridgeInstanceId: "bridge_123456789012345"
  };
  assert.equal(bridge.validEnvelope(flow, "START"), true);
  assert.equal(bridge.sameFlow(flow, flow), true);
  assert.equal(bridge.sameFlow({ ...flow, direction: "extension-to-page" }, flow), true);
  assert.equal(bridge.sameFlow({ ...flow, nonce: "nonce_abcdefghijklmnop" }, flow), false);
  assert.equal(bridge.validEnvelope({ ...flow, v: bridge.VERSION + 1 }), false);
});

test("only a safe raw _vcrcs value is returned to the page", () => {
  assert.deepEqual(bridge.cursorCredential({ value: "safe-token", expirationDate: 1_800_000_000 }), {
    value: "safe-token",
    expirationDate: 1_800_000_000
  });
  assert.deepEqual(bridge.cursorCredential({ value: "session-token" }), { value: "session-token" });
  for (const cookie of [
    null,
    { value: "" },
    { value: " leading" },
    { value: "secret; injected=1" },
    { value: "line\nbreak" }
  ]) assert.equal(bridge.cursorCredential(cookie), null);
});

test("worker uses the one-cookie API and never persists or logs credential material", () => {
  const worker = read("service-worker.js");
  assert.match(worker, /chrome\.cookies\.get\(\{ url: "https:\/\/cursor\.com\/", name: "_vcrcs" \}\)/);
  assert.equal(worker.includes("cookies." + "getAll"), false);
  assert.equal(/console\.(?:log|info|warn|error)/.test(worker), false);
  assert.equal(/storage\.(?:local|sync)\.set/.test(worker), false);
  assert.match(worker, /"RESULT", \{ credential \}/);
  const persistedBlock = worker.slice(worker.indexOf("const flow = {"), worker.indexOf("await setFlow(flow)"));
  assert.equal(persistedBlock.includes("cookie.value"), false);
  assert.equal(persistedBlock.includes("credential"), false);
});

test("content bridge validates same-window messages and does not inspect page fields", () => {
  const source = read("page-bridge.js");
  assert.equal(/querySelector|getElementById|password|document\.cookie/.test(source), false);
  assert.match(source, /event\.source !== window/);
  assert.match(source, /event\.origin !== window\.location\.origin/);
  assert.match(source, /navigator\.userActivation\.isActive/);
});
