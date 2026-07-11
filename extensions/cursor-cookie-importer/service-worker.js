"use strict";

importScripts("shared.js");

const bridge = globalThis.Sub2APICursorBridge;
const SESSION_KEY = "activeCursorImportFlow";
const ALARM_NAME = "cursor-import-timeout";
let queue = Promise.resolve();

chrome.storage.session.setAccessLevel({ accessLevel: "TRUSTED_CONTEXTS" }).catch(() => {});

function enqueue(task) {
  queue = queue.then(task, task).catch(() => {});
}

async function getFlow() {
  const stored = await chrome.storage.session.get(SESSION_KEY);
  return stored[SESSION_KEY] || null;
}

async function setFlow(flow) {
  await chrome.storage.session.set({ [SESSION_KEY]: flow });
}

async function clearFlow() {
  await chrome.storage.session.remove(SESSION_KEY);
  await chrome.alarms.clear(ALARM_NAME);
}

function senderOrigin(sender) {
  if (typeof sender.origin === "string" && sender.origin) return sender.origin;
  try {
    return new URL(sender.url || "").origin;
  } catch {
    return "";
  }
}

function isFixedOrigin(origin) {
  try {
    const url = new URL(origin);
    if (url.protocol === "https:" && (url.hostname === "www.poke2api.com" || url.hostname === "www.pokeapi.top")) return true;
    return (url.protocol === "http:" || url.protocol === "https:") &&
      (url.hostname === "localhost" || url.hostname === "127.0.0.1");
  } catch {
    return false;
  }
}

async function isAuthorizedSource(origin) {
  if (isFixedOrigin(origin)) return true;
  if (!origin.startsWith("https://")) return false;
  return chrome.permissions.contains({ origins: [origin + "/*"] });
}

function sourceFlow(message, sender) {
  return {
    channel: bridge.CHANNEL,
    v: bridge.VERSION,
    flowId: message.flowId,
    nonce: message.nonce,
    bridgeInstanceId: message.bridgeInstanceId,
    sourceTabId: sender.tab.id,
    sourceWindowId: sender.tab.windowId,
    sourceDocumentId: sender.documentId,
    sourceOrigin: senderOrigin(sender)
  };
}

function matchesSender(flow, sender) {
  return Boolean(sender.tab &&
    sender.frameId === 0 &&
    sender.tab.id === flow.sourceTabId &&
    sender.documentId === flow.sourceDocumentId &&
    senderOrigin(sender) === flow.sourceOrigin);
}

async function sendToSource(flow, type, extra) {
  const message = bridge.publicFlowMessage(type, flow, extra);
  try {
    await chrome.tabs.sendMessage(flow.sourceTabId, message, {
      frameId: 0,
      documentId: flow.sourceDocumentId
    });
    return true;
  } catch {
    return false;
  }
}

async function closeCursorTab(flow) {
  if (!Number.isInteger(flow.cursorTabId)) return;
  try {
    await chrome.tabs.remove(flow.cursorTabId);
  } catch {
    // The user may already have closed the tab.
  }
}

async function focusSource(flow) {
  try {
    await chrome.tabs.update(flow.sourceTabId, { active: true });
    if (Number.isInteger(flow.sourceWindowId)) {
      await chrome.windows.update(flow.sourceWindowId, { focused: true });
    }
  } catch {
    // The source tab or window may no longer exist.
  }
}

async function finish(flow, type, extra, options = {}) {
  await clearFlow();
  const delivered = await sendToSource(flow, type, extra);
  if (options.closeCursorTab !== false) await closeCursorTab(flow);
  if (delivered && options.focusSource) await focusSource(flow);
}

async function readAndDeliver(flow) {
  const current = await getFlow();
  if (!current || !bridge.sameFlow(current, flow)) return false;
  if (Date.now() >= current.expiresAt) {
    await finish(current, "ERROR", { code: "TIMEOUT" });
    return false;
  }

  let cookie;
  try {
    cookie = await chrome.cookies.get({ url: "https://cursor.com/", name: "_vcrcs" });
  } catch {
    await finish(current, "ERROR", { code: "INTERNAL_ERROR" });
    return false;
  }
  const credential = bridge.cursorCredential(cookie);
  if (!credential) return false;

  await sendToSource(current, "STATUS", { status: "reading_cookie" });
  await finish(current, "RESULT", { credential }, { focusSource: true });
  return true;
}

async function startFlow(message, sender) {
  if (!bridge.validEnvelope(message, "START")) return;
  if (!sender.tab || !Number.isInteger(sender.tab.id) || sender.frameId !== 0 || !sender.documentId) return;

  const requestedFlow = sourceFlow(message, sender);
  if (!(await isAuthorizedSource(requestedFlow.sourceOrigin))) {
    await sendToSource(requestedFlow, "ERROR", { code: "ORIGIN_NOT_ALLOWED" });
    return;
  }

  const existing = await getFlow();
  if (existing) {
    if (Date.now() >= existing.expiresAt) {
      await finish(existing, "ERROR", { code: "TIMEOUT" });
    } else {
      await sendToSource(requestedFlow, "ERROR", { code: "BUSY" });
      return;
    }
  }

  let cursorTab;
  try {
    cursorTab = await chrome.tabs.create({ url: "https://cursor.com/", active: true });
  } catch {
    await sendToSource(requestedFlow, "ERROR", { code: "INTERNAL_ERROR" });
    return;
  }
  if (!Number.isInteger(cursorTab.id)) {
    await sendToSource(requestedFlow, "ERROR", { code: "INTERNAL_ERROR" });
    return;
  }

  const startedAt = Date.now();
  const flow = {
    ...requestedFlow,
    cursorTabId: cursorTab.id,
    startedAt,
    expiresAt: startedAt + bridge.FLOW_TIMEOUT_MS
  };
  await setFlow(flow);
  chrome.alarms.create(ALARM_NAME, { when: flow.expiresAt });
  await sendToSource(flow, "ACCEPTED");
  await sendToSource(flow, "STATUS", { status: "waiting_for_login" });
  await readAndDeliver(flow);
}

async function cancelFlow(message, sender) {
  const flow = await getFlow();
  if (!flow || !bridge.sameFlow(message, flow) || !matchesSender(flow, sender)) return;
  await finish(flow, "ERROR", { code: message.type === "TIMEOUT" ? "TIMEOUT" : "CANCELLED" });
}

chrome.runtime.onMessage.addListener((message, sender) => {
  if (!message || message.channel !== bridge.CHANNEL || message.v !== bridge.VERSION) return;
  if (message.direction !== "page-to-extension") return;
  if (message.type === "START") enqueue(() => startFlow(message, sender));
  else if (message.type === "CANCEL" || message.type === "TIMEOUT") enqueue(() => cancelFlow(message, sender));
});

chrome.cookies.onChanged.addListener((changeInfo) => {
  const cookie = changeInfo && changeInfo.cookie;
  if (changeInfo.removed || !cookie || cookie.name !== "_vcrcs") return;
  const domain = String(cookie.domain || "").replace(/^\./, "").toLowerCase();
  if (domain !== "cursor.com" && !domain.endsWith(".cursor.com")) return;
  enqueue(async () => {
    const flow = await getFlow();
    if (flow) await readAndDeliver(flow);
  });
});

chrome.tabs.onRemoved.addListener((tabId) => {
  enqueue(async () => {
    const flow = await getFlow();
    if (!flow) return;
    if (tabId === flow.cursorTabId) {
      await finish(flow, "ERROR", { code: "CURSOR_TAB_CLOSED" }, { closeCursorTab: false });
    } else if (tabId === flow.sourceTabId) {
      await clearFlow();
      await closeCursorTab(flow);
    }
  });
});

chrome.tabs.onUpdated.addListener((tabId, changeInfo) => {
  if (changeInfo.status !== "loading" && changeInfo.status !== "complete") return;
  enqueue(async () => {
    const flow = await getFlow();
    if (!flow) return;
    if (changeInfo.status === "loading" && tabId === flow.sourceTabId) {
      await clearFlow();
      await closeCursorTab(flow);
    } else if (changeInfo.status === "complete" && tabId === flow.cursorTabId) {
      await readAndDeliver(flow);
    }
  });
});

chrome.alarms.onAlarm.addListener((alarm) => {
  if (alarm.name !== ALARM_NAME) return;
  enqueue(async () => {
    const flow = await getFlow();
    if (flow) await finish(flow, "ERROR", { code: "TIMEOUT" });
  });
});
