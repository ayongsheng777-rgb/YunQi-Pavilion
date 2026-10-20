/**
 * 云栖阁 · IT&AI 日报小组件前端工具
 * 后端接口：
 * - GET /api/itdaily/latest → 日报 JSON（JWT），无数据时返回 { exists: false }
 */

export interface ITDailyItem {
  idx: number;
  title: string;
  url: string;
  brief: string;
  subcategory?: string;
  impact?: string;
  advice?: string;
}

export interface ITDailySection {
  name: string;
  items: ITDailyItem[];
}

export interface ITDailyBug {
  title: string;
  url: string;
  problem: string;
  solution: string;
}

export interface ITDailyReport {
  exists: boolean;
  reportDate?: string;
  count?: number;
  sections?: ITDailySection[];
  bugs?: ITDailyBug[];
  trend?: string;
  receivedAt?: string;
}

const getToken = (): string => {
  try {
    return localStorage.getItem("flat-nas-token") || "";
  } catch {
    return "";
  }
};

export async function fetchITDailyLatest(): Promise<ITDailyReport> {
  const headers: Record<string, string> = {};
  const token = getToken();
  if (token) headers["Authorization"] = `Bearer ${token}`;
  const resp = await fetch("/api/itdaily/latest", { headers });
  if (!resp.ok) throw new Error(`获取日报失败: ${resp.status}`);
  return (await resp.json()) as ITDailyReport;
}
