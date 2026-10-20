<script setup lang="ts">
import { onMounted } from "vue";
import type { WidgetConfig } from "@/types";
import { useMusicPlayer } from "@/stores/musicPlayer";
import { QUALITIES } from "@/utils/music";
import MusicPanel from "@/components/MusicPanel.vue";

defineProps<{ widget: WidgetConfig }>();

const player = useMusicPlayer();
const {
  current,
  isPlaying,
  currentTime,
  duration,
  volume,
  resolving,
  coverUrl,
  playError,
  modeLabel,
  quality,
  currentLyric,
  progressPct,
} = player;

const fmtTime = (s: number) => {
  if (!s || isNaN(s)) return "00:00";
  const m = Math.floor(s / 60);
  const sec = Math.floor(s % 60);
  return `${String(m).padStart(2, "0")}:${String(sec).padStart(2, "0")}`;
};

const seek = (e: MouseEvent) => {
  const el = e.currentTarget as HTMLElement;
  const rect = el.getBoundingClientRect();
  player.seekRatio((e.clientX - rect.left) / rect.width);
};

const onVolume = (e: Event) => {
  player.setVolume(Number((e.target as HTMLInputElement).value));
};

onMounted(() => {
  void player.initPlayer();
});
</script>

<template>
  <div class="w-full h-full flex flex-col text-white select-none overflow-hidden">
    <!-- 主卡片 -->
    <div class="flex items-center gap-3 px-3 pt-3">
      <div
        class="w-14 h-14 rounded-xl overflow-hidden bg-white/10 flex items-center justify-center shrink-0"
      >
        <img v-if="coverUrl" :src="coverUrl" class="w-full h-full object-cover" alt="" />
        <span v-else class="text-2xl">🎵</span>
      </div>
      <div class="flex-1 min-w-0">
        <div class="text-sm font-bold truncate">{{ current ? current.name : "云栖阁音乐" }}</div>
        <div class="text-xs text-white/60 truncate">
          {{ current ? current.singer : "LX 引擎 · 导入的音源" }}
        </div>
        <div class="text-[11px] text-white/50 truncate h-4">
          {{ currentLyric || (resolving ? "正在解析播放地址…" : playError) }}
        </div>
      </div>
      <button
        @click="player.openPanel('search')"
        class="px-3 py-1.5 rounded-full bg-white/15 hover:bg-white/25 text-xs font-bold shrink-0"
      >
        点歌
      </button>
    </div>
    <!-- 进度条 -->
    <div class="px-3 mt-2">
      <div class="h-1.5 rounded-full bg-white/15 cursor-pointer" @click="seek">
        <div
          class="h-full rounded-full bg-white/80 transition-all"
          :style="{ width: progressPct + '%' }"
        ></div>
      </div>
      <div class="flex justify-between text-[10px] text-white/50 mt-0.5">
        <span>{{ fmtTime(currentTime) }}</span>
        <span>{{ current?.interval || fmtTime(duration) }}</span>
      </div>
    </div>
    <!-- 控制行 -->
    <div class="flex items-center gap-2 px-3 pb-3 mt-auto">
      <button
        @click="player.step(-1)"
        class="w-8 h-8 rounded-full bg-white/10 hover:bg-white/20 text-sm"
      >
        ⏮
      </button>
      <button
        @click="player.togglePlay"
        class="w-10 h-10 rounded-full bg-white text-gray-900 hover:bg-white/90 text-base font-bold"
      >
        {{ isPlaying ? "⏸" : "▶" }}
      </button>
      <button
        @click="player.step(1)"
        class="w-8 h-8 rounded-full bg-white/10 hover:bg-white/20 text-sm"
      >
        ⏭
      </button>
      <button
        @click="player.cycleMode()"
        class="px-2 py-1 rounded-full bg-white/10 hover:bg-white/20 text-[11px]"
        title="切换模式：只播放 / 边播边下 / 只下载"
      >
        {{ modeLabel }}
      </button>
      <select
        v-model="quality"
        class="bg-white/10 text-[11px] rounded-full px-2 py-1 outline-none [&>option]:text-gray-900"
      >
        <option v-for="q in QUALITIES" :key="q.id" :value="q.id">{{ q.name }}</option>
      </select>
      <input
        type="range"
        min="0"
        max="1"
        step="0.05"
        :value="volume"
        @input="onVolume"
        class="w-14 accent-white hidden sm:block"
      />
    </div>

    <MusicPanel />
  </div>
</template>
