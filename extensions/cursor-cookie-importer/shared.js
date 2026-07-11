(function (root, factory) {
  const api = factory();
  if (typeof module === "object" && module.exports) module.exports = api;
  root.Sub2APICursorBridge = api;
})(typeof globalThis !== "undefined" ? globalThis : this, function () {
  "use strict";

  const CHANNEL = "sub2api.cursor-login";
  const VERSION = 1;
  const FLOW_TIMEOUT_MS = 10 * 60 * 1000;
  const FIXED_ORIGINS = new Set([
    "https://www.poke2api.com",
    "https://www.pokeapi.top",
    "http://localhost",
    "https://localhost",
    "http://127.0.0.1",
    "https://127.0.0.1"
  ]);
  const TOKEN_PATTERN = /^[A-Za-z0-9_-]{16,128}$/;

  function isToken(value) {
    return typeof value === "string" && TOKEN_PATTERN.test(value);
  }

  function makeToken(byteLength) {
    const bytes = new Uint8Array(byteLength || 24);
    crypto.getRandomValues(bytes);
    let binary = "";
    for (const byte of bytes) binary += String.fromCharCode(byte);
    return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/g, "");
  }

  function normalizeOptionalOrigin(input) {
    if (typeof input !== "string") throw new Error("Origin must be a string");
    const value = input.trim();
    const parsed = new URL(value);
    if (parsed.protocol !== "https:") throw new Error("Self-hosted origins must use HTTPS");
    if (parsed.username || parsed.password) throw new Error("Origin must not contain credentials");
    if (parsed.pathname !== "/" || parsed.search || parsed.hash) throw new Error("Enter an exact origin without a path, query, or fragment");
    if (FIXED_ORIGINS.has(parsed.origin)) throw new Error("This origin is already supported");
    return parsed.origin;
  }

  function originPattern(origin) {
    return normalizeOptionalOrigin(origin) + "/*";
  }

  function validEnvelope(message, expectedType) {
    return Boolean(message &&
      message.channel === CHANNEL &&
      message.v === VERSION &&
      message.direction === "page-to-extension" &&
      (!expectedType || message.type === expectedType) &&
      isToken(message.flowId) &&
      isToken(message.nonce) &&
      isToken(message.bridgeInstanceId));
  }

  function sameFlow(message, flow) {
    return Boolean(message && flow &&
      message.channel === CHANNEL &&
      message.v === VERSION &&
      isToken(message.flowId) &&
      isToken(message.nonce) &&
      isToken(message.bridgeInstanceId) &&
      message.flowId === flow.flowId &&
      message.nonce === flow.nonce &&
      message.bridgeInstanceId === flow.bridgeInstanceId);
  }

  function cursorCredential(cookie) {
    if (!cookie || typeof cookie.value !== "string") return null;
    const value = cookie.value;
    if (!value || value.length > 16 * 1024 || value !== value.trim()) return null;
    for (const character of value) {
      const code = character.charCodeAt(0);
      if (code <= 0x20 || code === 0x7f || character === ";") return null;
    }
    const credential = { value };
    if (typeof cookie.expirationDate === "number" && Number.isFinite(cookie.expirationDate) && cookie.expirationDate > 0) {
      credential.expirationDate = cookie.expirationDate;
    }
    return credential;
  }

  function publicFlowMessage(type, flow, extra) {
    return Object.assign({
      channel: CHANNEL,
      v: VERSION,
      direction: "extension-to-page",
      type,
      flowId: flow.flowId,
      nonce: flow.nonce,
      bridgeInstanceId: flow.bridgeInstanceId
    }, extra || {});
  }

  return {
    CHANNEL,
    VERSION,
    FLOW_TIMEOUT_MS,
    FIXED_ORIGINS,
    isToken,
    makeToken,
    normalizeOptionalOrigin,
    originPattern,
    validEnvelope,
    sameFlow,
    cursorCredential,
    publicFlowMessage
  };
});
