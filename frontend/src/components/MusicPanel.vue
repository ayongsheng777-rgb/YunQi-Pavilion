<script setup lang="ts">
import { ref, onUnmounted, watch } from "vue";
import { useMusicPlayer } from "@/stores/musicPlayer";
import {
  QUALITIES,
  searchAllSources,
  deletePlaylistItem,
  type MusicInfo,
  type PlaylistItem,
} from "@/utils/music";

const player = useMusicPlayer();
const {
  panelOpen,
  panelTab,
  engineOk,
  loadedSourceCount,
  downloadTasks,
  playlist,
  current,
  notice,
  modeLabel,
  quality,
} = player;

const fmtSize = (n: number) =>
  n > 1024 * 1024 ? (n / 1024 / 1024).toFixed(1) + " MB" : Math.round(n / 1024) + " KB";

const onPlayLocal = (item: PlaylistItem) => {
  player.playPlaylistItem(item);
  panelOpen.value = false;
};

const onDeleteLocal = async (item: PlaylistItem) => {
  try {
    await deletePlaylistItem(item.id);
    player.showNotice("已删除：" + item.name);
    void player.refreshPlaylist();
  } catch (e) {
    player.showNotice("删除失败：" + (e as Error).message);
  }
};

// ---------- 搜索（聚合：五家曲库一起搜，界面不分平台） ----------
const searchQuery = ref("");
const searchResults = ref<MusicInfo[]>([]);
const searching = ref(false);
const searchPage = ref(1);
const searchHasMore = ref(false);

const doSearch = async (reset = true) => {
  const q = searchQuery.value.trim();
  if (!q) return;
  if (reset) {
    searchPage.value = 1;
    searchResults.value = [];
  }
  searching.value = true;
  try {
    const { list, hasMore } = await searchAllSources(q, searchPage.value, 20);
    searchResults.value = reset ? list : [...searchResults.value, ...list];
    searchHasMore.value = hasMore;
  } catch (e) {
    player.showNotice("搜索失败：" + (e as Error).message);
  } finally {
    searching.value = false;
  }
};
const loadMore = () => {
  searchPage.value += 1;
  void doSearch(false);
};

// ---------- 下载 ----------
const taskPct = (t: { total: number; progress: number; status: string }) =>
  t.total > 0
    ? Math.min(100, Math.round((t.progress / t.total) * 100))
    : t.status === "done"
      ? 100
      : 0;

let pollTimer: ReturnType<typeof setInterval> | null = null;
watch(panelOpen, (open) => {
  if (open) {
    void player.refreshTasks();
    pollTimer = setInterval(() => {
      if (panelTab.value === "downloads") void player.refreshTasks();
    }, 3000);
  } else if (pollTimer) {
    clearInterval(pollTimer);
    pollTimer = null;
  }
});
onUnmounted(() => {
  if (pollTimer) clearInterval(pollTimer);
});

const onPlay = (info: MusicInfo) => {
  void player.playTrack(info, searchResults.value);
  panelOpen.value = false;
};
</script>

<template>
  <div
    v-if="panelOpen"
    class="fixed inset-0 z-[9000] flex items-end sm:items-center justify-center bg-black/60"
    @click.self="panelOpen = false"
  >
    <div
      class="w-full sm:w-[640px] max-h-[85vh] flex flex-col rounded-t-2xl sm:rounded-2xl bg-gray-900 text-white overflow-hidden"
    >
      <div class="flex items-center gap-2 px-4 py-3 border-b border-white/10">
        <span class="font-bold">🎵 点歌台</span>
        <span v-if="engineOk === false" class="text-[11px] text-red-300"
          >引擎未连接（到设置里检查音乐配置）</span
        >
        <span v-else-if="engineOk" class="text-[11px] text-white/50"
          >已加载音源 {{ loadedSourceCount }} 个</span
        >
        <button
          @click="player.cycleMode()"
          class="ml-auto px-2 py-1 rounded-full bg-white/10 hover:bg-white/20 text-[11px]"
          title="切换模式：只播放 / 边播边下 / 只下载"
        >
          {{ modeLabel }}
        </button>
        <select
          v-model="quality"
          class="bg-white/10 text-[11px] rounded-full px-2 py-1 outline-none [&>option]:text-gray-900"
          title="音质"
        >
          <option v-for="q in QUALITIES" :key="q.id" :value="q.id">{{ q.name }}</option>
        </select>
        <button
          @click="panelOpen = false"
          class="w-8 h-8 rounded-full bg-white/10 hover:bg-white/20 shrink-0"
        >
          ✕
        </button>
      </div>

      <div class="flex gap-2 px-4 pt-3">
        <button
          v-for="t in [
            { id: 'playlist', n: '歌单' },
            { id: 'search', n: '搜索' },
            { id: 'downloads', n: '下载' },
          ]"
          :key="t.id"
          @click="panelTab = t.id as typeof panelTab"
          class="px-3 py-1.5 rounded-full text-xs font-bold"
          :class="panelTab === t.id ? 'bg-white text-gray-900' : 'bg-white/10 hover:bg-white/20'"
        >
          {{ t.n }}
        </button>
      </div>

      <div
        v-if="notice"
        class="mx-4 mt-2 px-3 py-2 rounded-lg bg-emerald-500/20 text-emerald-200 text-xs"
      >
        {{ notice }}
      </div>

      <!-- 歌单页：已下载的歌曲 -->
      <div
        v-if="panelTab === 'playlist'"
        class="flex flex-col min-h-0 px-4 py-3 overflow-y-auto"
      >
        <div v-if="!playlist.length" class="text-center text-white/40 text-sm py-6">
          还没有下载的歌。去搜索页点 ⬇ 下载，下完自动进歌单，在这里点开就能放（放的是 NAS 上的文件，不用再解析）
        </div>
        <div
          v-for="it in playlist"
          :key="it.id"
          class="flex items-center gap-2 px-2 py-2 rounded-lg hover:bg-white/5 cursor-pointer"
          :class="current && current.source === 'local' && current.songmid === it.id ? 'bg-white/10' : ''"
          @click="onPlayLocal(it)"
        >
          <div class="flex-1 min-w-0">
            <div class="text-sm truncate">
              {{ it.name }}
              <span
                v-if="it.hasLyric"
                class="text-[10px] px-1.5 py-0.5 rounded bg-sky-500/30 text-sky-200 align-middle"
                >词</span
              >
            </div>
            <div class="text-xs text-white/50 truncate">
              {{ it.singer }} · {{ it.quality || "" }} · {{ fmtSize(it.size) }}
            </div>
          </div>
          <button
            @click.stop="onDeleteLocal(it)"
            title="从 NAS 删除这首歌"
            class="w-7 h-7 rounded-full bg-white/10 hover:bg-red-500/60 text-xs shrink-0"
          >
            🗑
          </button>
        </div>
        <button
          @click="player.refreshPlaylist"
          class="mt-2 py-2 text-xs text-white/60 hover:text-white"
        >
          刷新
        </button>
      </div>

      <!-- 搜索页 -->
      <div v-if="panelTab === 'search'" class="flex flex-col min-h-0 px-4 py-3">
        <div class="flex gap-2">
          <input
            v-model="searchQuery"
            @keyup.enter="doSearch()"
            placeholder="输入歌名 / 歌手，回车搜索"
            class="flex-1 min-w-0 bg-white/10 rounded-lg px-3 py-2 text-sm placeholder-white/40 outline-none focus:bg-white/15"
          />
          <button
            @click="doSearch()"
            class="px-4 py-2 rounded-lg bg-white text-gray-900 text-sm font-bold"
          >
            搜索
          </button>
        </div>
        <div class="overflow-y-auto mt-3 min-h-0">
          <div v-if="searching" class="text-center text-white/50 text-sm py-6">搜索中…</div>
          <div v-else-if="!searchResults.length" class="text-center text-white/40 text-sm py-6">
            搜首歌试试，播放只用你导入的音源解析
          </div>
          <div
            v-for="it in searchResults"
            :key="it.source + '-' + it.songmid"
            class="flex items-center gap-2 px-2 py-2 rounded-lg hover:bg-white/5 cursor-pointer"
            @click="onPlay(it)"
          >
            <div class="flex-1 min-w-0">
              <div class="text-sm truncate">
                {{ it.name }}
                <span class="text-white/40 text-xs">{{ it.interval }}</span>
              </div>
              <div class="text-xs text-white/50 truncate">{{ it.singer }} · {{ it.albumName }}</div>
            </div>
            <span
              v-for="ty in (it.types || []).slice(0, 3)"
              :key="ty.type"
              class="text-[10px] px-1.5 py-0.5 rounded bg-white/10 text-white/60"
              >{{ ty.type }}</span
            >
            <button
              @click.stop="player.downloadTrack(it)"
              title="下载到 NAS"
              class="w-7 h-7 rounded-full bg-white/10 hover:bg-white/25 text-xs shrink-0"
            >
              ⬇
            </button>
          </div>
          <button
            v-if="searchHasMore"
            @click="loadMore"
            class="w-full py-2 text-xs text-white/60 hover:text-white"
          >
            加载更多
          </button>
        </div>
      </div>

      <!-- 下载页 -->
      <div v-if="panelTab === 'downloads'" class="flex flex-col min-h-0 px-4 py-3 overflow-y-auto">
        <div v-if="!downloadTasks.length" class="text-center text-white/40 text-sm py-6">
          还没有下载记录。点歌曲旁边的 ⬇ 就会下载到 NAS 目录
        </div>
        <div v-for="t in downloadTasks" :key="t.id" class="px-2 py-2">
          <div class="flex items-center gap-2">
            <div class="flex-1 min-w-0">
              <div class="text-sm truncate">{{ t.name }} - {{ t.singer }}</div>
              <div class="text-[11px] text-white/50">
                {{ t.quality }} ·
                {{
                  t.status === "done"
                    ? "已完成：" + t.file
                    : t.status === "failed"
                      ? "失败：" + (t.error || "")
                      : "下载中…"
                }}
              </div>
            </div>
            <span class="text-xs text-white/60">{{ taskPct(t) }}%</span>
          </div>
          <div class="h-1.5 rounded-full bg-white/10 mt-1">
            <div
              class="h-full rounded-full"
              :class="t.status === 'failed' ? 'bg-red-400' : 'bg-emerald-400'"
              :style="{ width: taskPct(t) + '%' }"
            ></div>
          </div>
        </div>
        <button
          @click="player.refreshTasks"
          class="mt-2 py-2 text-xs text-white/60 hover:text-white"
        >
          刷新
        </button>
      </div>
    </div>
  </div>
</template>
