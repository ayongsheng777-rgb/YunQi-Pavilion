// 云栖阁收藏 - 后台 service worker：右键菜单"收藏到云栖阁"
importScripts("config.js");

const DEFAULT_CATEGORY = "浏览器收藏"; // 右键快速收藏用的分类（云栖阁没有会自动新建）

chrome.runtime.onInstalled.addListener(() => {
  chrome.contextMenus.create({
    id: "yq-save-page",
    title: "收藏到云栖阁",
    contexts: ["page"],
  });
  chrome.contextMenus.create({
    id: "yq-save-link",
    title: "收藏这个链接到云栖阁",
    contexts: ["link"],
  });
});

// 在网页里弹一个小提示条（3 秒自动消失）
function yqInjectToast(tabId, text, ok) {
  chrome.scripting.executeScript({
    target: { tabId },
    func: (msg, isOk) => {
      const old = document.getElementById("yq-toast");
      if (old) old.remove();
      const el = document.createElement("div");
      el.id = "yq-toast";
      el.textContent = msg;
      el.style.cssText =
        "position:fixed;top:16px;left:50%;transform:translateX(-50%);z-index:2147483647;" +
        "padding:10px 18px;border-radius:8px;font-size:14px;color:#fff;box-shadow:0 2px 12px rgba(0,0,0,.3);" +
        "background:" + (isOk ? "#1a9e54" : "#c0392b") + ";";
      document.body.appendChild(el);
      setTimeout(() => el.remove(), 3000);
    },
    args: [text, ok],
  }).catch(() => {});
}

chrome.contextMenus.onClicked.addListener(async (info, tab) => {
  const { serverUrl, token } = await yqGetConfig();
  if (!serverUrl || !token) {
    if (tab && tab.id) {
      yqInjectToast(tab.id, "请先配置云栖阁：点浏览器工具栏的云栖阁图标 → 右键 → 选项，填好服务器地址和 Token", false);
    }
    return;
  }

  const isLink = info.menuItemId === "yq-save-link";
  const url = (isLink ? info.linkUrl : info.pageUrl) || "";
  const title = (isLink ? info.linkText : tab ? tab.title : "") || url;

  try {
    const resp = await fetch(yqApi(serverUrl, "/bookmark/create"), {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "Authorization": "Bearer " + token,
      },
      body: JSON.stringify({ title, url, description: "", category: DEFAULT_CATEGORY }),
    });
    const data = await resp.json().catch(() => ({}));
    if (resp.ok && data.success) {
      if (tab && tab.id) yqInjectToast(tab.id, "已收藏到云栖阁", true);
    } else if (resp.status === 401) {
      if (tab && tab.id) yqInjectToast(tab.id, "Token 无效，请去选项页重新填写（云栖阁设置 → 插件与 AI 助手的钥匙）", false);
    } else {
      if (tab && tab.id) yqInjectToast(tab.id, "收藏失败：" + (data.error || "服务器没响应"), false);
    }
  } catch (e) {
    if (tab && tab.id) yqInjectToast(tab.id, "连不上云栖阁服务器，请检查服务器地址和网络", false);
  }
});
