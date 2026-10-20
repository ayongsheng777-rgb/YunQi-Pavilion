/**
 * 云栖阁 · 音乐（LX Music 集成）前端工具
 * 后端接口（JWT 鉴权）：
 * - GET  /api/music/config                     音乐配置
 * - POST /api/music/config                     保存配置
 * - GET  /api/music/sources                    音源脚本列表
 * - POST /api/music/sources/import             { url } 或 { code } 导入音源脚本
 * - DELETE /api/music/sources/:id              删除音源脚本
 * - GET  /api/music/search?source=&q=&page=&limit=
 * - POST /api/music/music-url                  { source, musicInfo, quality }
 * - GET  /api/music/lyric?source=&musicInfo=
 * - GET  /api/music/pic?source=&musicInfo=
 * - GET  /api/music/leaderboard?source=
 * - GET  /api/music/leaderboard/songs?source=&id=
 * - GET  /api/music/hot-search?source=
 * - POST /api/music/download                   { source, musicInfo, quality }
 * - GET  /api/music/download/tasks
 */

export interface MusicInfo {
  name: string;
  singer: string;
  source: string;
  songmid: string | number;
  albumName?: string;
  albumId?: string | number;
  interval?: string;
  img?: string | null;
  types?: { type: string; size?: string }[];
  [key: string]: unknown;
}

export interface MusicConfig {
  engineUrl: string;
  proxyEnabled: boolean;
  proxyHost: string;
  proxyPort: number;
  downloadDir: string;
  defaultSource: string;
  defaultQuality: string;
  defaultMode: string;
}

export interface SourceScript {
  id: string;
  name?: string;
  version?: string;
  author?: string;
  ok: boolean;
  error?: string;
  sources?: Record<string, { actions: string[]; qualitys: string[] }>;
}

export interface DownloadTask {
  id: string;
  name: string;
  singer: string;
  source: string;
  quality: string;
  status: string;
  progress: number;
  total: number;
  file: string;
  error?: string;
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

async function api<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, { ...init, headers: authHeaders() });
  if (!res.ok) throw new Error(`请求失败（${res.status}）`);
  return (await res.json()) as T;
}

// 引擎返回统一包 { code, msg, data }
interface EngineResp<T> {
  code: number;
  msg?: string;
  data: T;
}

async function engine<T>(url: string, init?: RequestInit): Promise<T> {
  const r = await api<EngineResp<T>>(url, init);
  if (r.code !== 0) throw new Error(r.msg || "音乐引擎出错");
  return r.data;
}

/** 曲库来源（内部用：搜歌时五家一起搜、合并去重，界面上不分平台） */
export const SEARCH_SOURCE_IDS = ["kw", "kg", "tx", "wy", "mg"];

export const QUALITIES = [
  { id: "128k", name: "128k" },
  { id: "320k", name: "320k" },
  { id: "flac", name: "FLAC" },
  { id: "flac24bit", name: "FLAC 24bit" },
];

export async function fetchMusicConfig(): Promise<MusicConfig> {
  const r = await api<{ config: MusicConfig }>("/api/music/config");
  return r.config;
}

export async function saveMusicConfig(cfg: MusicConfig): Promise<void> {
  await api("/api/music/config", { method: "POST", body: JSON.stringify(cfg) });
}

export async function fetchSources(): Promise<SourceScript[]> {
  return engine<SourceScript[]>("/api/music/sources");
}

export async function importSource(url: string): Promise<SourceScript[]> {
  return engine<SourceScript[]>("/api/music/sources/import", {
    method: "POST",
    body: JSON.stringify({ url }),
  });
}

export async function deleteSource(id: string): Promise<SourceScript[]> {
  return engine<SourceScript[]>(`/api/music/sources/${encodeURIComponent(id)}`, { method: "DELETE" });
}

export async function searchMusic(source: string, q: string, page = 1, limit = 30): Promise<{ list: MusicInfo[]; total?: number; allPage?: number }> {
  const data = await engine<{ list: MusicInfo[]; total?: number; allPage?: number }>(
    `/api/music/search?source=${encodeURIComponent(source)}&q=${encodeURIComponent(q)}&page=${page}&limit=${limit}`
  );
  return { list: Array.isArray(data.list) ? data.list : [], total: data.total, allPage: data.allPage };
}

const normKey = (s: string) => (s || "").toLowerCase().replace(/[\s\-_·・'"'"'’‘“”《》<>[\]()（）]/g, "");

/**
 * 聚合搜索：五家曲库一起搜，交错合并并按「歌名+歌手」去重。
 * 界面只呈现一个搜索框，播放仍只走用户导入的音源脚本。
 */
export async function searchAllSources(
  q: string,
  page = 1,
  perSource = 20
): Promise<{ list: MusicInfo[]; hasMore: boolean }> {
  const settled = await Promise.allSettled(
    SEARCH_SOURCE_IDS.map((id) => searchMusic(id, q, page, perSource))
  );
  const lists = settled.map((r) => (r.status === "fulfilled" ? r.value.list : []));
  const seen = new Set<string>();
  const merged: MusicInfo[] = [];
  const maxLen = Math.max(0, ...lists.map((l) => l.length));
  for (let i = 0; i < maxLen; i++) {
    for (const l of lists) {
      const it = l[i];
      if (!it) continue;
      const key = normKey(it.name) + "|" + normKey(it.singer);
      if (seen.has(key)) continue;
      seen.add(key);
      merged.push(it);
    }
  }
  return { list: merged, hasMore: lists.some((l) => l.length >= perSource) };
}

export async function fetchMusicUrl(source: string, musicInfo: MusicInfo, quality: string): Promise<string> {
  const data = await engine<{ url: string }>("/api/music/music-url", {
    method: "POST",
    body: JSON.stringify({ source, musicInfo, quality }),
  });
  return data.url;
}

export async function fetchLyric(source: string, musicInfo: MusicInfo): Promise<{ lyric?: string; tlyric?: string } | null> {
  try {
    return await engine(`/api/music/lyric?source=${encodeURIComponent(source)}&musicInfo=${encodeURIComponent(JSON.stringify(musicInfo))}`);
  } catch {
    return null;
  }
}

export async function fetchPic(source: string, musicInfo: MusicInfo): Promise<string> {
  try {
    const data = await engine<string>(`/api/music/pic?source=${encodeURIComponent(source)}&musicInfo=${encodeURIComponent(JSON.stringify(musicInfo))}`);
    return typeof data === "string" ? data : "";
  } catch {
    return "";
  }
}

export async function startDownload(source: string, musicInfo: MusicInfo, quality: string): Promise<void> {
  await api("/api/music/download", {
    method: "POST",
    body: JSON.stringify({ source, musicInfo, quality }),
  });
}

export async function fetchDownloadTasks(): Promise<DownloadTask[]> {
  const r = await api<{ tasks: DownloadTask[] }>("/api/music/download/tasks");
  return Array.isArray(r.tasks) ? r.tasks : [];
}
