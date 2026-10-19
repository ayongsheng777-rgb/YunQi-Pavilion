// 云栖阁收藏 - 弹窗逻辑
document.addEventListener("DOMContentLoaded", init);

function showMsg(text, ok) {
  const el = document.getElementById("msg");
  el.textContent = text;
  el.className = ok ? "ok" : "err";
}

async function init() {
  const { serverUrl, token } = await yqGetConfig();

  document.getElementById("goOptions").addEventListener("click", () => chrome.runtime.openOptionsPage());
  document.getElementById("openOptions").addEventListener("click", () => chrome.runtime.openOptionsPage());

  // 没配好：只显示配置提示
  if (!serverUrl || !token) {
    document.getElementById("needConfig").style.display = "block";
    document.getElementById("form").style.display = "none";
    return;
  }

  // 取当前标签页的标题和网址
  try {
    const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
    if (tab) {
      document.getElementById("title").value = tab.title || "";
      document.getElementById("url").value = tab.url || "";
    }
  } catch (e) { /* 取不到就留空让用户手填 */ }

  // 拉分类列表：GET /openapi/v1/categories
  const sel = document.getElementById("category");
  try {
    const resp = await fetch(yqApi(serverUrl, "/categories"), {
      headers: { "Authorization": "Bearer " + token },
    });
    if (resp.status === 401) {
      showMsg("Token 无效，请去「选项设置」里重新填写（Token 在云栖阁 设置 → 插件与 AI 助手的钥匙 里查看）", false);
      return;
    }
    if (!resp.ok) throw new Error("http " + resp.status);
    const data = await resp.json();
    const cats = (data && data.categories) || [];
    sel.innerHTML = "";
    cats.forEach((c) => {
      const opt = document.createElement("option");
      opt.value = c.title;
      opt.textContent = c.title + "（" + c.count + "）";
      sel.appendChild(opt);
    });
  } catch (e) {
    showMsg("连不上云栖阁服务器，请检查选项里的服务器地址和网络", false);
    return;
  }
  // 最后一项：新建分类
  const optNew = document.createElement("option");
  optNew.value = "__new__";
  optNew.textContent = "＋ 新建分类…";
  sel.appendChild(optNew);

  sel.addEventListener("change", () => {
    document.getElementById("newCatWrap").style.display =
      sel.value === "__new__" ? "block" : "none";
  });

  document.getElementById("save").addEventListener("click", saveBookmark);
}

async function saveBookmark() {
  const { serverUrl, token } = await yqGetConfig();
  const title = document.getElementById("title").value.trim();
  const url = document.getElementById("url").value.trim();
  const description = document.getElementById("desc").value.trim();
  const sel = document.getElementById("category");
  let category = sel.value;
  if (category === "__new__") {
    category = document.getElementById("newCat").value.trim();
  }

  if (!title) { showMsg("标题不能为空", false); return; }
  if (!url) { showMsg("网址不能为空", false); return; }
  if (!category) { showMsg("请选择分类，或新建一个分类", false); return; }

  const btn = document.getElementById("save");
  btn.disabled = true;
  btn.textContent = "收藏中…";
  try {
    // POST /openapi/v1/bookmark/create
    const resp = await fetch(yqApi(serverUrl, "/bookmark/create"), {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "Authorization": "Bearer " + token,
      },
      body: JSON.stringify({ title, url, description, category }),
    });
    const data = await resp.json().catch(() => ({}));
    if (resp.ok && data.success) {
      showMsg("收藏成功，已存到「" + category + "」", true);
    } else if (resp.status === 401) {
      showMsg("Token 无效，请去「选项设置」里重新填写", false);
    } else {
      showMsg("收藏失败：" + (data.error || "服务器没响应"), false);
    }
  } catch (e) {
    showMsg("连不上云栖阁服务器，请检查服务器地址和网络", false);
  } finally {
    btn.disabled = false;
    btn.textContent = "收 藏";
  }
}
