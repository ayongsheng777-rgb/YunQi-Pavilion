<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from "vue";
import type { WidgetConfig } from "@/types";
import { useMainStore } from "../stores/main";

interface VpsContainer { name: string; state: string; image: string }
interface VpsNodeStatus {
  id: string; name: string; host: string; online: boolean; error?: string;
  latencyMs: number; updatedAt: number; os: string; cores: number;
  cpuPercent: number; memPercent: number; memUsed: number; memTotal: number;
  diskPercent: number; diskUsed: number; diskTotal: number;
  load1: number; uptimeSec: number; netUpRate: number; netDownRate: number;
  sshLogAvailable: boolean; sshFailed24h: number; sshLastIp: string;
  containers: VpsContainer[]; containersRunning: number; containersTotal: number;
  proxyContainer: string;
}

const props = defineProps<{ widget: WidgetConfig }>();
const store = useMainStore();

const nodes = ref<VpsNodeStatus[]>([]);
const loaded = ref(false);
const loadError = ref("");
let timer: ReturnType<typeof setInterval> | null = null;

const isCompact = computed(() => (props.widget.h ?? props.widget.rowSpan ?? 1) <= 1);
const updatedText = computed(() => {
  const t = Math.max(0, ...nodes.value.map((n) => n.updatedAt || 0));
  if (!t) return "";
  const sec = Math.max(0, Math.round((Date.now() - t) / 1000));
  return sec < 5 ? "刚刚更新" : `${sec} 秒前更新`;
});

const getToken = (): string => {
  try { return localStorage.getItem("flat-nas-token") || ""; } catch { return ""; }
};

const reload = async () => {
  if (!store.isLogged) { loaded.value = true; return; }
  try {
    const resp = await fetch("/api/vps/status", {
      headers: { Authorization: `Bearer ${getToken()}` },
      cache: "no-store",
    });
    if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
    const j = await resp.json();
    nodes.value = (j.nodes ?? []) as VpsNodeStatus[];
    loadError.value = "";
  } catch (e) {
    loadError.value = e instanceof Error ? e.message : "加载失败";
  } finally {
    loaded.value = true;
  }
};

onMounted(() => {
  void reload();
  timer = setInterval(reload, 20000);
});
onUnmounted(() => { if (timer) clearInterval(timer); });

const pct = (v: number) => (Math.round(v * 10) / 10).toString();
const barColor = (v: number) => (v >= 85 ? "#f87171" : v >= 60 ? "#f59e0b" : "#7dd3a8");
const fmtGB = (b: number) => (b / 1073741824).toFixed(1) + "G";
const fmtRate = (b: number) => {
  if (!b || b < 1024) return "0 KB/s";
  if (b < 1048576) return (b / 1024).toFixed(0) + " KB/s";
  return (b / 1048576).toFixed(1) + " MB/s";
};
const fmtUptime = (s: number) => {
  if (!s) return "-";
  const d = Math.floor(s / 86400), h = Math.floor((s % 86400) / 3600);
  return d > 0 ? `${d} 天 ${h} 小时` : `${h} 小时`;
};
const CIRC = 2 * Math.PI * 9;
const ringOffset = (v: number) => CIRC * (1 - Math.min(100, Math.max(0, v)) / 100);

// 每台挑一个「关键服务」展示：优先代理/网关类容器
const keyService = (n: VpsNodeStatus): VpsContainer | null => {
  const keys = ["sing-box", "xray", "v2ray", "hysteria", "trojan", "nginx", "openresty"];
  return n.containers?.find((c) => keys.some((k) => c.name.toLowerCase().includes(k))) ?? null;
};
</script>

<template>
  <div
    class="w-full h-full rounded-2xl backdrop-blur border border-white/10 overflow-hidden flex flex-col text-white relative"
    :style="{
      backgroundColor: `rgba(0,0,0,${Math.min(0.85, Math.max(0.15, widget.opacity ?? 0.35))})`,
      color: '#fff',
    }"
  >
    <div class="px-4 py-2.5 border-b border-white/10 flex items-center justify-between bg-white/10 shrink-0">
      <div class="flex items-center gap-2">
        <span class="text-base">🖥️</span>
        <span class="font-bold text-sm">VPS 监控</span>
        <span v-if="nodes.length" class="text-[11px] text-white/50">{{ nodes.filter(n => n.online).length }}/{{ nodes.length }} 在线</span>
      </div>
      <div class="flex items-center gap-2">
        <span v-if="updatedText" class="text-[11px] text-white/40">{{ updatedText }}</span>
        <button class="text-xs bg-white/10 text-white/70 px-2 py-0.5 rounded hover:bg-white/20 hover:text-white" @click="reload" title="刷新">↻</button>
      </div>
    </div>

    <div class="flex-1 overflow-y-auto p-3 min-h-0">
      <div v-if="!loaded" class="text-white/50 text-sm px-1 py-2">加载中…</div>
      <div v-else-if="!store.isLogged" class="text-white/60 text-sm px-1 py-2">登录后可见</div>
      <div v-else-if="loadError && !nodes.length" class="text-red-300 text-sm px-1 py-2">数据加载失败：{{ loadError }}</div>
      <div v-else-if="!nodes.length" class="text-white/60 text-sm px-1 py-2 leading-relaxed">
        还没有添加 VPS 节点。<br />到「设置 → 扩展」页的「VPS 监控（1Panel）」卡里填上面板地址和 API 密钥即可。
      </div>

      <!-- 文件袋卡片：每台服务器一张 -->
      <div v-else class="flex flex-wrap gap-x-3 gap-y-4 pt-1">
        <div v-for="n in nodes" :key="n.id" class="relative" style="flex: 1 1 235px; min-width: 225px;">
          <!-- 标签头 -->
          <div
            class="absolute -top-[13px] left-3 z-10 flex items-center gap-1.5 pl-3 pr-5 py-[3px] text-xs font-bold text-white/90"
            :style="{
              background: 'rgba(24,34,40,0.92)',
              border: `1px solid ${n.online ? 'rgba(110,210,160,0.45)' : 'rgba(248,113,113,0.55)'}`,
              borderBottom: 'none',
              borderRadius: '8px 10px 0 0',
              clipPath: 'polygon(0 0, 100% 0, calc(100% - 9px) 100%, 0 100%)',
            }"
          >
            <span :class="n.online ? 'text-green-400' : 'text-red-400'">●</span>{{ n.name }}
          </div>
          <!-- 卡体 -->
          <div
            class="rounded-lg px-3 pb-2.5 pt-[15px] h-full"
            :style="{
              background: 'rgba(16,24,29,0.55)',
              border: `1px solid ${n.online ? 'rgba(110,210,160,0.28)' : 'rgba(248,113,113,0.45)'}`,
            }"
          >
            <template v-if="n.online">
              <!-- 仪表行（紧凑/完整共用） -->
              <div class="flex items-center gap-3">
                <div v-for="g in [
                  { lb: 'CPU', v: n.cpuPercent },
                  { lb: '内存', v: n.memPercent },
                  { lb: '磁盘', v: n.diskPercent },
                ]" :key="g.lb" class="flex items-center gap-1.5">
                  <svg width="24" height="24" viewBox="0 0 24 24" class="shrink-0 -rotate-90">
                    <circle cx="12" cy="12" r="9" fill="none" stroke="rgba(255,255,255,0.14)" stroke-width="2.5" />
                    <circle cx="12" cy="12" r="9" fill="none" :stroke="barColor(g.v)" stroke-width="2.5" stroke-linecap="round"
                      :stroke-dasharray="CIRC" :stroke-dashoffset="ringOffset(g.v)" />
                  </svg>
                  <span class="text-[11px] text-white/60">{{ g.lb }}</span>
                  <span class="text-xs font-bold text-white/90">{{ pct(g.v) }}%</span>
                </div>
                <span class="ml-auto text-[11px] text-white/40">{{ n.latencyMs }}ms</span>
              </div>
              <!-- 服务行 -->
              <div class="flex items-center justify-between mt-1.5 text-xs">
                <span v-if="keyService(n)" :class="keyService(n)!.state === 'running' ? 'text-green-400' : 'text-red-400'">
                  {{ keyService(n)!.name }} {{ keyService(n)!.state === 'running' ? '运行中' : keyService(n)!.state }}
                </span>
                <span v-else class="text-white/60">容器 {{ n.containersRunning }}/{{ n.containersTotal }} 运行中</span>
                <span v-if="n.sshLogAvailable" :class="n.sshFailed24h >= 500 ? 'text-red-400 font-bold' : 'text-white/55'">
                  🛡️ 爆破 {{ n.sshFailed24h }} 次
                </span>
              </div>
              <!-- 完整模式的明细 -->
              <template v-if="!isCompact">
                <div class="flex justify-between items-center bg-black/25 rounded-md px-2 py-1.5 text-[11.5px] mt-2">
                  <span>🐳 容器</span>
                  <span class="text-white/85">{{ n.containersRunning }}/{{ n.containersTotal }} 运行中</span>
                </div>
                <div class="flex justify-between items-center bg-black/25 rounded-md px-2 py-1.5 text-[11.5px] mt-1.5">
                  <span>🌐 网络</span>
                  <span class="text-white/85">↓ {{ fmtRate(n.netDownRate) }} · ↑ {{ fmtRate(n.netUpRate) }}</span>
                </div>
                <div v-if="n.sshLogAvailable" class="flex justify-between items-center bg-black/25 rounded-md px-2 py-1.5 text-[11.5px] mt-1.5">
                  <span>🛡️ SSH 爆破（24h）</span>
                  <span class="text-white/85">{{ n.sshFailed24h }} 次<span v-if="n.sshLastIp" class="text-white/45"> · 最近 {{ n.sshLastIp }}</span></span>
                </div>
                <div class="flex justify-between text-[11px] text-white/45 px-0.5 mt-2">
                  <span>{{ n.os }}<span v-if="n.cores"> · {{ n.cores }} 核</span></span>
                  <span>负载 {{ n.load1.toFixed(2) }} · 已运行 {{ fmtUptime(n.uptimeSec) }}</span>
                </div>
                <div class="flex justify-between text-[11px] text-white/35 px-0.5 mt-0.5">
                  <span>{{ n.host }}</span>
                  <span>内存 {{ fmtGB(n.memUsed) }}/{{ fmtGB(n.memTotal) }} · 磁盘 {{ fmtGB(n.diskUsed) }}/{{ fmtGB(n.diskTotal) }}</span>
                </div>
              </template>
            </template>
            <div v-else class="pt-1">
              <div class="text-red-300 text-xs font-bold">● 离线</div>
              <div class="text-red-200/70 text-[11px] leading-relaxed break-all mt-0.5">{{ n.error || '连接失败' }}</div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
