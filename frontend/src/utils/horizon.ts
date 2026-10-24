/**
 * 云栖阁 · Horizon 日报前端工具
 * 后端接口：
 * - GET /api/horizon/list → 日期列表（JWT）
 * - GET /api/horizon/day?date=YYYY-MM-DD → 当天全文 Markdown（JWT，不传取最新）
 * 推送方：OECT NAS 每天 08:00 跑完 Horizon 后 POST /openapi/v1/horizon/push。
 */

import DOMPurify from "dompurify";

export interface HorizonIndex {
  exists: boolean;
  dates?: string[];
  latest?: string;
  counts?: Record<string, number>;
}

export interface HorizonDay {
  exists: boolean;
  date?: string;
  markdown?: string;
}

const getToken = (): string => {
  try {
    return localStorage.getItem("flat-nas-token") || "";
  } catch {
    return "";
  }
};

const authHeaders = (): Record<string, string> => {
  const headers: Record<string, string> = {};
  const token = getToken();
  if (token) headers["Authorization"] = `Bearer ${token}`;
  return headers;
};

export async function fetchHorizonList(): Promise<HorizonIndex> {
  const resp = await fetch("/api/horizon/list", { headers: authHeaders() });
  if (!resp.ok) throw new Error(`获取日报列表失败: ${resp.status}`);
  return (await resp.json()) as HorizonIndex;
}

export async function fetchHorizonDay(date?: string): Promise<HorizonDay> {
  const url = date ? `/api/horizon/day?date=${encodeURIComponent(date)}` : "/api/horizon/day";
  const resp = await fetch(url, { headers: authHeaders() });
  if (!resp.ok) throw new Error(`获取日报全文失败: ${resp.status}`);
  return (await resp.json()) as HorizonDay;
}

/** 从日报 Markdown 里抽 ### 标题行（卡片上显示用）。 */
export function extractHeadlines(md: string): string[] {
  const out: string[] = [];
  for (const line of md.split("\n")) {
    const m = line.match(/^### \[(.+?)\]\(/);
    if (m) out.push(m[1]);
  }
  return out;
}

function escapeHtml(s: string): string {
  return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;");
}

/** 行内：粗体、行内代码、链接（站外新窗口打开，站内锚点保留）。 */
function renderInline(text: string): string {
  let out = escapeHtml(text);
  out = out.replace(/`([^`]+)`/g, "<code>$1</code>");
  out = out.replace(/\*\*([^*]+)\*\*/g, "<strong>$1</strong>");
  out = out.replace(/\[([^\]]+)\]\(([^)\s]+)\)/g, (_m, label: string, href: string) => {
    if (href.startsWith("#")) return `<a href="${href}">${label}</a>`;
    return `<a href="${href}" target="_blank" rel="noopener">${label}</a>`;
  });
  return out;
}

/**
 * 够用的小 Markdown 渲染：只覆盖 Horizon 日报实际用到的写法
 *（标题、分隔线、引用、无序列表、段落、行内格式、<a id> 锚点），
 * 输出再过 DOMPurify，不怕正文里夹带奇怪 HTML。
 */
export function renderHorizonMarkdown(md: string): string {
  const lines = md.split("\n");
  const html: string[] = [];
  let inList = false;
  const closeList = () => {
    if (inList) {
      html.push("</ul>");
      inList = false;
    }
  };

  for (const raw of lines) {
    const line = raw.trimEnd();
    const trimmed = line.trim();
    if (trimmed === "") {
      closeList();
      continue;
    }
    const anchor = trimmed.match(/^<a id="([^"]+)"><\/a>$/);
    if (anchor) {
      closeList();
      html.push(`<span id="${escapeHtml(anchor[1])}"></span>`);
      continue;
    }
    if (trimmed === "---") {
      closeList();
      html.push("<hr />");
      continue;
    }
    const h = trimmed.match(/^(#{1,3})\s+(.*)$/);
    if (h) {
      closeList();
      const level = h[1].length;
      html.push(`<h${level}>${renderInline(h[2])}</h${level}>`);
      continue;
    }
    if (trimmed.startsWith("- ")) {
      if (!inList) {
        html.push("<ul>");
        inList = true;
      }
      html.push(`<li>${renderInline(trimmed.slice(2))}</li>`);
      continue;
    }
    if (trimmed.startsWith("> ")) {
      closeList();
      html.push(`<blockquote>${renderInline(trimmed.slice(2))}</blockquote>`);
      continue;
    }
    closeList();
    html.push(`<p>${renderInline(trimmed)}</p>`);
  }
  closeList();
  return DOMPurify.sanitize(html.join("\n"), { ADD_ATTR: ["target"] });
}
