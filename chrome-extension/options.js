// 云栖阁收藏 - 选项页逻辑
document.addEventListener("DOMContentLoaded", async () => {
  const { serverUrl, token } = await yqGetConfig();
  document.getElementById("serverUrl").value = serverUrl;
  document.getElementById("token").value = token;

  document.getElementById("save").addEventListener("click", async () => {
    const s = document.getElementById("serverUrl").value;
    const t = document.getElementById("token").value;
    if (!s.trim()) { showMsg("请填写服务器地址", false); return; }
    if (!/^https?:\/\//i.test(s.trim())) { showMsg("服务器地址要以 http:// 或 https:// 开头", false); return; }
    if (!t.trim()) { showMsg("请填写 Token", false); return; }
    await yqSaveConfig(s, t);
    showMsg("已保存", true);
  });

  document.getElementById("test").addEventListener("click", async () => {
    const s = document.getElementById("serverUrl").value.trim().replace(/\/+$/, "");
    const t = document.getElementById("token").value.trim();
    if (!s || !t) { showMsg("请先填好服务器地址和 Token 再测试", false); return; }
    showMsg("正在连接…", true);
    try {
      // GET /openapi/v1/version：连通性 + Token 有效性一次验证
      const resp = await fetch(yqApi(s, "/version"), {
        headers: { "Authorization": "Bearer " + t },
      });
      if (resp.status === 401) { showMsg("连上了服务器，但 Token 无效，请重新复制", false); return; }
      if (!resp.ok) { showMsg("服务器返回异常（" + resp.status + "）", false); return; }
      const data = await resp.json();
      showMsg("连接成功：" + (data.name || "云栖阁") + " " + (data.version || ""), true);
    } catch (e) {
      showMsg("连不上服务器，请检查地址是否正确、NAS 是否开机、网络是否通", false);
    }
  });
});

function showMsg(text, ok) {
  const el = document.getElementById("msg");
  el.textContent = text;
  el.className = ok ? "ok" : "err";
}
