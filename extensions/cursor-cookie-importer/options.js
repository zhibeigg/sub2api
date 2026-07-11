(() => {
  "use strict";

  const bridge = globalThis.Sub2APICursorBridge;
  const STORAGE_KEY = "optionalOrigins";
  const SCRIPT_PREFIX = "sub2api-origin-";
  const form = document.querySelector("#origin-form");
  const input = document.querySelector("#origin");
  const list = document.querySelector("#origins");
  const status = document.querySelector("#status");

  function scriptId(origin) {
    let hash = 2166136261;
    for (let index = 0; index < origin.length; index += 1) {
      hash ^= origin.charCodeAt(index);
      hash = Math.imul(hash, 16777619);
    }
    return SCRIPT_PREFIX + (hash >>> 0).toString(16);
  }

  async function readOrigins() {
    const stored = await chrome.storage.local.get(STORAGE_KEY);
    return Array.isArray(stored[STORAGE_KEY]) ? stored[STORAGE_KEY] : [];
  }

  async function saveOrigins(origins) {
    await chrome.storage.local.set({ [STORAGE_KEY]: origins.slice().sort() });
  }

  async function register(origin) {
    const id = scriptId(origin);
    try {
      await chrome.scripting.unregisterContentScripts({ ids: [id] });
    } catch {
      // The registration may not exist yet.
    }
    await chrome.scripting.registerContentScripts([{
      id,
      matches: [bridge.originPattern(origin)],
      js: ["shared.js", "page-bridge.js"],
      runAt: "document_start",
      persistAcrossSessions: true
    }]);
  }

  async function render() {
    const origins = await readOrigins();
    list.replaceChildren();
    for (const origin of origins) {
      const item = document.createElement("li");
      const code = document.createElement("code");
      code.textContent = origin;
      const remove = document.createElement("button");
      remove.type = "button";
      remove.textContent = "Remove";
      remove.addEventListener("click", async () => {
        status.textContent = "";
        await chrome.scripting.unregisterContentScripts({ ids: [scriptId(origin)] }).catch(() => {});
        await chrome.permissions.remove({ origins: [bridge.originPattern(origin)] });
        await saveOrigins((await readOrigins()).filter((value) => value !== origin));
        await render();
      });
      item.append(code, remove);
      list.append(item);
    }
  }

  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    status.className = "";
    try {
      const origin = bridge.normalizeOptionalOrigin(input.value);
      const granted = await chrome.permissions.request({ origins: [bridge.originPattern(origin)] });
      if (!granted) throw new Error("Host permission was not granted");
      await register(origin);
      const origins = await readOrigins();
      if (!origins.includes(origin)) await saveOrigins([...origins, origin]);
      input.value = "";
      status.textContent = "Access granted for " + origin;
      await render();
    } catch (error) {
      status.className = "error";
      status.textContent = error instanceof Error ? error.message : "Unable to add origin";
    }
  });

  render().catch(() => {
    status.className = "error";
    status.textContent = "Unable to load saved origins";
  });
})();
