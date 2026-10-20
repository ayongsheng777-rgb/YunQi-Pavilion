/**
 * 云栖阁 · 签到相关前端工具
 * 后端接口契约（另一位工程师并行实现，前端按此调用）：
 * - GET  /api/checkin/sites          → { sites: [...] }（JWT）
 * - POST /api/checkin/sites          → { op: add|update|delete, ... }（JWT）
 * - POST /api/checkin/open           → { url } 或 { site_id }（JWT）
 * - GET  /api/checkin/go/:id         → 浏览器直接打开即 302 跳转（先标记已签到）
 * - GET  /api/checkin/status         → { date, status: [{ site_id, name, url, done }] }（JWT）
 */

export interface CheckinSite {
  id: string;
  name: string;
  url: string;
  icon?: string;
  note?: string;
  enabled: boolean;
}

export interface CheckinStatusItem {
  site_id: string;
  name: string;
  url: string;
  done: boolean;
}

const getToken = (): string => {
  try {
    return localStorage.getItem("flat-nas-token") || "";
  } catch {
    return "";
  }
};

const authHeaders = (): Record<string, string> => {
  const headers: Record<string, string> = { "Content-Type": "application/json" };
  const token = getToken();
  if (token) headers["Authorization"] = `Bearer ${token}`;
  return headers;
};

const isHttpUrl = (url: string): boolean => /^https?:\/\//i.test(url || "");

/**
 * 外链点击时调用：先通知后端"通过导航打开过"，失败也不阻塞跳转。
 * 只在 http/https 外链上触发。
 */
export function trackCheckinOpen(url: string): void {
  if (!url || !isHttpUrl(url)) return;
  try {
    // fire-and-forget：签到追踪绝不影响正常跳转
    void fetch("/api/checkin/open", {
      method: "POST",
      headers: authHeaders(),
      body: JSON.stringify({ url }),
    }).catch(() => undefined);
  } catch {
    // ignore
  }
}

/** 在新窗口打开签到站点的后端跳转地址（后端 302 并自动标记已签到）
 * 浏览器直接打开带不了 Authorization 请求头，所以把 JWT 放到 ?token= 查询参数里
 * （后端 AuthMiddleware 明确支持从 query 取 token；302 跳转到外部站点时不会带上它）。
 * 没登录（无 token）时就裸开，靠可信设备 cookie，失败后端会返回 401 说明。 */
export function openCheckinSite(siteId: string): void {
  if (!siteId) return;
  const token = getToken();
  const url =
    `/api/checkin/go/${encodeURIComponent(siteId)}` +
    (token ? `?token=${encodeURIComponent(token)}` : "");
  window.open(url, "_blank");
}

/** 拉取签到站点列表（需要登录） */
export async function fetchCheckinSites(): Promise<CheckinSite[]> {
  const res = await fetch("/api/checkin/sites", { headers: authHeaders() });
  if (!res.ok) throw new Error(`拉取签到站点失败（${res.status}）`);
  const data = await res.json();
  return Array.isArray(data?.sites) ? data.sites : [];
}

/** 在已收藏书签里搜索，供签到站点表单自动匹配 */
export interface BookmarkMatch {
  id: string;
  title: string;
  url: string;
  icon?: string;
  category?: string;
}

export async function searchCheckinBookmarks(q: string): Promise<BookmarkMatch[]> {
  const query = (q || "").trim();
  if (!query) return [];
  const res = await fetch(`/api/checkin/search?q=${encodeURIComponent(query)}`, { headers: authHeaders() });
  if (!res.ok) throw new Error(`搜索失败（${res.status}）`);
  const data = await res.json();
  return Array.isArray(data?.results) ? data.results : [];
}

/** 拉取今日签到状态（需要登录） */
export async function fetchCheckinStatus(): Promise<CheckinStatusItem[]> {
  const res = await fetch("/api/checkin/status", { headers: authHeaders() });
  if (!res.ok) throw new Error(`拉取签到状态失败（${res.status}）`);
  const data = await res.json();
  return Array.isArray(data?.status) ? data.status : [];
}

export async function addCheckinSite(site: Omit<CheckinSite, "id">): Promise<CheckinSite> {
  const res = await fetch("/api/checkin/sites", {
    method: "POST",
    headers: authHeaders(),
    body: JSON.stringify({
      op: "add",
      site: { name: site.name, url: site.url, icon: site.icon || "", note: site.note || "", enabled: site.enabled !== false },
    }),
  });
  if (!res.ok) throw new Error("添加站点失败");
  const data = await res.json();
  if (!data?.success) throw new Error(data?.error || "添加站点失败");
  return data.site as CheckinSite;
}

export async function updateCheckinSite(site: CheckinSite): Promise<void> {
  const res = await fetch("/api/checkin/sites", {
    method: "POST",
    headers: authHeaders(),
    body: JSON.stringify({ op: "update", site }),
  });
  if (!res.ok) throw new Error("保存站点失败");
  const data = await res.json();
  if (!data?.success) throw new Error(data?.error || "保存站点失败");
}

export async function deleteCheckinSite(siteId: string): Promise<void> {
  const res = await fetch("/api/checkin/sites", {
    method: "POST",
    headers: authHeaders(),
    body: JSON.stringify({ op: "delete", id: siteId }),
  });
  if (!res.ok) throw new Error("删除站点失败");
  const data = await res.json();
  if (!data?.success) throw new Error(data?.error || "删除站点失败");
}
