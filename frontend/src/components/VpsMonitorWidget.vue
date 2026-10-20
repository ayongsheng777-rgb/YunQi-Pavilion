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
const barColor = (v: number) => (v >= 85 ? "#f87171" : v >= 60 ? "#f59e0b" : "#60a5fa");
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

      <!-- 紧凑模式（高度 1） -->
      <div v-else-if="isCompact" class="flex flex-col gap-1.5">
        <div
          v-for="n in nodes" :key="n.id"
          class="flex items-center gap-3 px-2.5 py-1.5 rounded-lg bg-white/5 border border-white/10 text-xs whitespace-nowrap overflow-hidden"
        >
          <span :class="n.online ? 'text-green-400' : 'text-red-400'">●</span>
          <span class="font-bold truncate max-w-[130px]">{{ n.name }}</span>
          <template v-if="n.online">
            <span class="text-white/60">CPU <b class="text-white/90">{{ pct(n.cpuPercent) }}%</b></span>
            <span class="text-white/60">内存 <b class="text-white/90">{{ pct(n.memPercent) }}%</b></span>
            <span class="text-white/60">磁盘 <b class="text-white/90">{{ pct(n.diskPercent) }}%</b></span>
            <span v-if="n.proxyContainer" :class="n.proxyContainer === 'running' ? 'text-green-400' : 'text-red-400'">sing-box {{ n.proxyContainer === 'running' ? '运行中' : n.proxyContainer }}</span>
            <span class="text-white/40 ml-auto">{{ n.latencyMs }}ms</span>
          </template>
          <span v-else class="text-red-300 truncate">离线 · {{ n.error || '连接失败' }}</span>
        </div>
      </div>

      <!-- 卡片模式 -->
      <div v-else class="grid gap-3" style="grid-template-columns: repeat(auto-fit, minmax(215px, 1fr));">
        <div
          v-for="n in nodes" :key="n.id"
          class="rounded-xl bg-white/5 border p-3 flex flex-col gap-2"
          :class="n.online ? 'border-white/10' : 'border-red-400/40'"
        >
          <div class="flex items-start justify-between gap-2">
            <div class="min-w-0">
              <div class="font-bold text-sm truncate">{{ n.name }}</div>
              <div class="text-[11px] text-white/45 truncate">{{ n.host }}<span v-if="n.os"> · {{ n.os }}</span></div>
            </div>
            <span class="text-[11px] font-bold shrink-0" :class="n.online ? 'text-green-400' : 'text-red-400'">{{ n.online ? '● 在线' : '● 离线' }}</span>
          </div>

          <template v-if="n.online">
            <div class="flex flex-col gap-1.5">
              <div v-for="bar in [
                { lb: 'CPU', v: n.cpuPercent, extra: n.cores ? n.cores + ' 核' : '' },
                { lb: '内存', v: n.memPercent, extra: fmtGB(n.memUsed) + '/' + fmtGB(n.memTotal) },
                { lb: '磁盘', v: n.diskPercent, extra: fmtGB(n.diskUsed) + '/' + fmtGB(n.diskTotal) },
              ]" :key="bar.lb" class="flex items-center gap-2 text-[11px]">
                <span class="w-7 text-white/55">{{ bar.lb }}</span>
                <div class="flex-1 h-1.5 rounded bg-white/10 overflow-hidden">
                  <div class="h-full rounded" :style="{ width: Math.min(100, bar.v) + '%', background: barColor(bar.v) }"></div>
                </div>
                <span class="w-11 text-right text-white/85">{{ pct(bar.v) }}%</span>
                <span class="w-[74px] text-right text-white/40">{{ bar.extra }}</span>
              </div>
            </div>
            <div class="flex justify-between items-center bg-black/25 rounded-md px-2 py-1.5 text-[11.5px]">
              <span>🚀 代理 sing-box</span>
              <span v-if="n.proxyContainer" :class="n.proxyContainer === 'running' ? 'text-green-400 font-bold' : 'text-red-400 font-bold'">{{ n.proxyContainer === 'running' ? '运行中' : n.proxyContainer }}</span>
              <span v-else class="text-white/50">未安装</span>
            </div>
            <div class="flex justify-between items-center bg-black/25 rounded-md px-2 py-1.5 text-[11.5px]">
              <span>🐳 容器</span>
              <span class="text-white/85">{{ n.containersRunning }}/{{ n.containersTotal }} 运行中</span>
            </div>
            <div class="flex justify-between items-center bg-black/25 rounded-md px-2 py-1.5 text-[11.5px]">
              <span>🌐 网络</span>
              <span class="text-white/85">↓ {{ fmtRate(n.netDownRate) }} · ↑ {{ fmtRate(n.netUpRate) }}</span>
            </div>
            <div class="flex justify-between text-[11px] text-white/45 px-0.5">
              <span>负载 {{ n.load1.toFixed(2) }} · 已运行 {{ fmtUptime(n.uptimeSec) }}</span>
              <span>{{ n.latencyMs }}ms</span>
            </div>
          </template>
          <div v-else class="text-red-300 text-xs leading-relaxed break-all">{{ n.error || '连接失败' }}</div>
        </div>
      </div>
    </div>
  </div>
</template>
