/**
 * 云栖阁 · LX 音乐播放器（共享状态）
 * 顶部播放器（原 MiniPlayer 位置）和音乐小组件卡片共用这一套状态与同一个音频元素，
 * 保证全站只有一处在放歌。播放地址只通过用户导入的音源脚本解析。
 */
import { ref, computed } from "vue";
import {
  fetchMusicConfig,
  fetchSources,
  fetchMusicUrl,
  fetchLyric,
  fetchPic,
  startDownload,
  fetchDownloadTasks,
  fetchPlaylist,
  fetchPlaylistLyric,
  localStreamUrl,
  type MusicInfo,
  type DownloadTask,
  type PlaylistItem,
} from "@/utils/music";

export type PlayMode = "play" | "download" | "playDownload";
export type PanelTab = "search" | "playlist" | "downloads";

interface LyricLine {
  time: number;
  text: string;
}

const parseLrc = (text: string): LyricLine[] => {
  const lines: LyricLine[] = [];
  for (const raw of (text || "").split("\n")) {
    const m = raw.match(/\[(\d+):(\d+(?:\.\d+)?)\](.*)/);
    if (m) lines.push({ time: Number(m[1]) * 60 + Number(m[2]), text: m[3].trim() });
  }
  return lines.sort((a, b) => a.time - b.time);
};

// ---------- 单例状态 ----------
let audioEl: HTMLAudioElement | null = null;
const queue = ref<MusicInfo[]>([]);
const currentIndex = ref(-1);
const isPlaying = ref(false);
const currentTime = ref(0);
const duration = ref(0);
const resolving = ref(false);
const coverUrl = ref("");
const playError = ref("");
const mode = ref<PlayMode>("play");
const quality = ref("320k");
const lyricLines = ref<LyricLine[]>([]);
const panelOpen = ref(false);
const panelTab = ref<PanelTab>("search");
const engineOk = ref<boolean | null>(null);
const loadedSourceCount = ref(0);
const downloadTasks = ref<DownloadTask[]>([]);
const playlist = ref<PlaylistItem[]>([]);
const lyricsOpen = ref(false);
const notice = ref("");

const current = computed<MusicInfo | null>(() =>
  currentIndex.value >= 0 ? queue.value[currentIndex.value] : null
);
const volume = ref(Number(localStorage.getItem("yunqi-music-volume") ?? 0.8) || 0.8);

const lyricIndex = computed(() => {
  let idx = -1;
  for (let i = 0; i < lyricLines.value.length; i++) {
    if (lyricLines.value[i].time <= currentTime.value) idx = i;
    else break;
  }
  return idx;
});
const currentLyric = computed(() =>
  lyricIndex.value >= 0 ? lyricLines.value[lyricIndex.value].text : ""
);
const modeLabel = computed(() =>
  mode.value === "play" ? "只播放" : mode.value === "download" ? "只下载" : "边播边下"
);
const progressPct = computed(() =>
  duration.value > 0 ? (currentTime.value / duration.value) * 100 : 0
);

let noticeTimer: ReturnType<typeof setTimeout> | null = null;
const showNotice = (msg: string) => {
  notice.value = msg;
  if (noticeTimer) clearTimeout(noticeTimer);
  noticeTimer = setTimeout(() => {
    if (notice.value === msg) notice.value = "";
  }, 3500);
};

const ensureAudio = (): HTMLAudioElement => {
  if (audioEl) return audioEl;
  audioEl = new Audio();
  audioEl.volume = volume.value;
  audioEl.addEventListener("timeupdate", () => {
    currentTime.value = audioEl?.currentTime || 0;
  });
  audioEl.addEventListener("loadedmetadata", () => {
    duration.value = audioEl?.duration || 0;
  });
  audioEl.addEventListener("play", () => {
    isPlaying.value = true;
  });
  audioEl.addEventListener("pause", () => {
    isPlaying.value = false;
  });
  audioEl.addEventListener("ended", () => {
    isPlaying.value = false;
    if (queue.value.length > 1) void step(1);
  });
  audioEl.addEventListener("error", () => {
    if (audioEl?.src) {
      playError.value = "播放出错，这首歌的地址可能失效了";
      isPlaying.value = false;
    }
  });
  return audioEl;
};

// ---------- 播放控制 ----------
const playTrack = async (info: MusicInfo, list?: MusicInfo[]) => {
  const a = ensureAudio();
  if (list) {
    queue.value = list;
    currentIndex.value = list.findIndex(
      (x) => x.songmid === info.songmid && x.source === info.source
    );
  } else if (
    currentIndex.value < 0 ||
    queue.value[currentIndex.value]?.songmid !== info.songmid ||
    queue.value[currentIndex.value]?.source !== info.source
  ) {
    if (!queue.value.some((x) => x.songmid === info.songmid && x.source === info.source)) {
      queue.value = [info];
    }
    currentIndex.value = queue.value.findIndex(
      (x) => x.songmid === info.songmid && x.source === info.source
    );
  }
  playError.value = "";

  // 本地已下载歌曲：直接放 NAS 上的文件，不走音源解析、也不触发下载
  if (info.source === "local") {
    coverUrl.value = "";
    lyricLines.value = [];
    const a = ensureAudio();
    a.src = localStreamUrl(String(info.songmid));
    try {
      await a.play();
      isPlaying.value = true;
    } catch {
      playError.value = "播放失败，这首本地歌曲可能被删了";
      isPlaying.value = false;
    }
    fetchPlaylistLyric(String(info.songmid)).then((text) => {
      if (text && current.value?.songmid === info.songmid) {
        lyricLines.value = parseLrc(text);
      }
    });
    return;
  }

  if (mode.value === "download" || mode.value === "playDownload") {
    try {
      await startDownload(info.source, info, quality.value);
      showNotice(`已开始下载：${info.name}`);
      void refreshTasks();
    } catch (e) {
      showNotice("下载启动失败：" + (e as Error).message);
    }
  }
  if (mode.value === "download") return;

  resolving.value = true;
  try {
    const url = await fetchMusicUrl(info.source, info, quality.value);
    a.src = url;
    await a.play();
    isPlaying.value = true;
    lyricLines.value = [];
    fetchLyric(info.source, info).then((r) => {
      if (r && r.lyric && current.value?.songmid === info.songmid) {
        lyricLines.value = parseLrc(r.lyric);
      }
    });
    coverUrl.value = info.img || "";
    if (!coverUrl.value) {
      fetchPic(info.source, info).then((p) => {
        if (p && current.value?.songmid === info.songmid) coverUrl.value = p;
      });
    }
  } catch (e) {
    playError.value = "播放失败：" + (e as Error).message;
    isPlaying.value = false;
  } finally {
    resolving.value = false;
  }
};

const togglePlay = async () => {
  const a = ensureAudio();
  if (!current.value) {
    panelOpen.value = true;
    panelTab.value = "search";
    return;
  }
  if (isPlaying.value) {
    a.pause();
  } else {
    if (!a.src) {
      await playTrack(current.value);
      return;
    }
    try {
      await a.play();
    } catch {
      playError.value = "播放失败，点下一首试试";
    }
  }
};

const step = async (dir: 1 | -1) => {
  if (!queue.value.length) return;
  const idx =
    (currentIndex.value + dir + queue.value.length) % queue.value.length;
  await playTrack(queue.value[idx]);
};

const seekRatio = (ratio: number) => {
  const a = ensureAudio();
  if (!duration.value) return;
  a.currentTime = Math.max(0, Math.min(1, ratio)) * duration.value;
};

const setVolume = (v: number) => {
  volume.value = v;
  localStorage.setItem("yunqi-music-volume", String(v));
  if (audioEl) audioEl.volume = v;
};

const cycleMode = () => {
  mode.value =
    mode.value === "play" ? "playDownload" : mode.value === "playDownload" ? "download" : "play";
  showNotice("模式：" + modeLabel.value);
};

// ---------- 下载与歌单 ----------
const refreshTasks = async () => {
  try {
    downloadTasks.value = await fetchDownloadTasks();
  } catch {
    /* 忽略 */
  }
  void refreshPlaylist();
};

const refreshPlaylist = async () => {
  try {
    playlist.value = await fetchPlaylist();
  } catch {
    /* 忽略 */
  }
};

/** 把歌单条目转成播放队列用的歌曲对象 */
const playlistAsQueue = (list: PlaylistItem[]): MusicInfo[] =>
  list.map((it) => ({
    name: it.name,
    singer: it.singer,
    source: "local",
    songmid: it.id,
    albumName: "已下载",
    types: [],
  }));

const playPlaylistItem = (item: PlaylistItem) => {
  const q = playlistAsQueue(playlist.value);
  const info = q.find((x) => x.songmid === item.id) || q[0];
  if (info) void playTrack(info, q);
};

const seekToTime = (sec: number) => {
  const a = ensureAudio();
  a.currentTime = Math.max(0, sec);
};

const downloadTrack = async (info: MusicInfo) => {
  try {
    await startDownload(info.source, info, quality.value);
    showNotice(`已开始下载：${info.name}`);
    void refreshTasks();
  } catch (e) {
    showNotice("下载启动失败：" + (e as Error).message);
  }
};

// ---------- 初始化 ----------
let inited = false;
const initPlayer = async () => {
  ensureAudio();
  if (inited) return;
  inited = true;
  try {
    const cfg = await fetchMusicConfig();
    if (cfg.defaultQuality) quality.value = cfg.defaultQuality;
    if (
      cfg.defaultMode === "download" ||
      cfg.defaultMode === "playDownload" ||
      cfg.defaultMode === "play"
    ) {
      mode.value = cfg.defaultMode;
    }
  } catch {
    /* 用默认值 */
  }
  try {
    const list = await fetchSources();
    loadedSourceCount.value = list.filter((s) => s.ok).length;
    engineOk.value = true;
  } catch {
    engineOk.value = false;
  }
  void refreshTasks();
};

const openPanel = (tab?: PanelTab) => {
  // 不指定页时：歌单里有歌就先看歌单，否则去搜索
  panelTab.value = tab ?? (playlist.value.length ? "playlist" : "search");
  panelOpen.value = true;
  void refreshPlaylist();
};

export function useMusicPlayer() {
  return {
    queue,
    currentIndex,
    current,
    isPlaying,
    currentTime,
    duration,
    volume,
    resolving,
    coverUrl,
    playError,
    mode,
    quality,
    lyricLines,
    currentLyric,
    panelOpen,
    panelTab,
    engineOk,
    loadedSourceCount,
    downloadTasks,
    playlist,
    lyricsOpen,
    lyricIndex,
    notice,
    modeLabel,
    progressPct,
    playTrack,
    togglePlay,
    step,
    seekRatio,
    seekToTime,
    setVolume,
    cycleMode,
    refreshTasks,
    refreshPlaylist,
    playPlaylistItem,
    downloadTrack,
    initPlayer,
    openPanel,
    showNotice,
  };
}
