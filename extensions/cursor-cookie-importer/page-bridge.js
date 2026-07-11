(() => {
  "use strict";

  const bridge = globalThis.Sub2APICursorBridge;
  const bridgeInstanceId = bridge.makeToken(24);
  let active = null;
  let timeoutId = null;

  function post(message) {
    window.postMessage(message, window.location.origin);
  }

  function postReady() {
    post({
      channel: bridge.CHANNEL,
      v: bridge.VERSION,
      direction: "extension-to-page",
      type: "READY",
      bridgeInstanceId,
      extensionVersion: chrome.runtime.getManifest().version
    });
  }

  function clearActive() {
    if (timeoutId !== null) clearTimeout(timeoutId);
    timeoutId = null;
    active = null;
  }

  function workerFlowMessage(type, flow) {
    return { ...bridge.publicFlowMessage(type, flow), direction: "page-to-extension" };
  }

  function sendToWorker(message) {
    chrome.runtime.sendMessage(message).catch(() => {
      if (active && bridge.sameFlow(message, active)) {
        post(bridge.publicFlowMessage("ERROR", active, { code: "INTERNAL_ERROR" }));
        clearActive();
      }
    });
  }

  window.addEventListener("message", (event) => {
    if (event.source !== window || event.origin !== window.location.origin) return;
    const message = event.data;
    if (!message || message.channel !== bridge.CHANNEL || message.v !== bridge.VERSION || message.direction !== "page-to-extension") return;
    if (message.type === "PING") {
      postReady();
      return;
    }
    if (message.bridgeInstanceId !== bridgeInstanceId) return;

    if (message.type === "START") {
      if (!bridge.validEnvelope(message, "START")) return;
      if (!navigator.userActivation || !navigator.userActivation.isActive) {
        post(bridge.publicFlowMessage("ERROR", message, { code: "INTERNAL_ERROR" }));
        return;
      }
      if (active) {
        post(bridge.publicFlowMessage("ERROR", message, { code: "BUSY" }));
        return;
      }
      active = {
        flowId: message.flowId,
        nonce: message.nonce,
        bridgeInstanceId
      };
      timeoutId = setTimeout(() => {
        if (!active) return;
        sendToWorker(workerFlowMessage("TIMEOUT", active));
        post(bridge.publicFlowMessage("ERROR", active, { code: "TIMEOUT" }));
        clearActive();
      }, bridge.FLOW_TIMEOUT_MS);
      sendToWorker(workerFlowMessage("START", active));
      return;
    }

    if ((message.type === "CANCEL" || message.type === "TIMEOUT") && active && bridge.sameFlow(message, active)) {
      sendToWorker(message);
      clearActive();
    }
  });

  chrome.runtime.onMessage.addListener((message) => {
    if (!active || !bridge.sameFlow(message, active)) return;
    if (!["ACCEPTED", "STATUS", "RESULT", "ERROR"].includes(message.type)) return;
    post(message);
    if (["RESULT", "ERROR"].includes(message.type)) clearActive();
  });

  postReady();
})();
