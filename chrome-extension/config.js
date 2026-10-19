// 云栖阁收藏 - 配置读写（popup 与 background 共用）
// 服务器地址存在 chrome.storage.sync（换电脑自动同步），Token 存在 chrome.storage.local（只留本机）

async function yqGetConfig() {
  const sync = await chrome.storage.sync.get(["serverUrl"]);
  const local = await chrome.storage.local.get(["token"]);
  const serverUrl = (sync.serverUrl || "").trim().replace(/\/+$/, "");
  const token = (local.token || "").trim();
  return { serverUrl, token };
}

async function yqSaveConfig(serverUrl, token) {
  serverUrl = (serverUrl || "").trim().replace(/\/+$/, "");
  token = (token || "").trim();
  await chrome.storage.sync.set({ serverUrl });
  await chrome.storage.local.set({ token });
}

// 拼出 OpenAPI 地址，例如 http://192.168.57.228:23000/openapi/v1/...
function yqApi(serverUrl, path) {
  return serverUrl + "/openapi/v1" + path;
}
