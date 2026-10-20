<script setup lang="ts">
import { ref, watch, nextTick } from "vue";
import { useMusicPlayer } from "@/stores/musicPlayer";

const player = useMusicPlayer();
const { lyricsOpen, lyricLines, lyricIndex, current, isPlaying } = player;
const listRef = ref<HTMLElement | null>(null);

watch(lyricIndex, async () => {
  if (!lyricsOpen.value) return;
  await nextTick();
  const el = listRef.value?.querySelector<HTMLElement>("[data-active='1']");
  el?.scrollIntoView({ block: "center", behavior: "smooth" });
});

const onLineClick = (time: number) => {
  player.seekToTime(time);
};
</script>

<template>
  <div
    v-if="lyricsOpen"
    class="fixed inset-0 z-[9001] flex items-end sm:items-center justify-center bg-black/60"
    @click.self="lyricsOpen = false"
  >
    <div
      class="w-full sm:w-[560px] max-h-[80vh] flex flex-col rounded-t-2xl sm:rounded-2xl bg-gray-900 text-white overflow-hidden"
    >
      <div class="flex items-center gap-2 px-4 py-3 border-b border-white/10">
        <span class="font-bold truncate">
          🎤 {{ current ? current.name + " - " + current.singer : "歌词" }}
        </span>
        <button
          @click="lyricsOpen = false"
          class="ml-auto w-8 h-8 rounded-full bg-white/10 hover:bg-white/20 shrink-0"
        >
          ✕
        </button>
      </div>
      <div ref="listRef" class="overflow-y-auto px-6 py-6 min-h-[200px]">
        <div v-if="!current" class="text-center text-white/40 text-sm py-8">先选一首歌来放</div>
        <div v-else-if="!lyricLines.length" class="text-center text-white/40 text-sm py-8">
          这首歌暂时没取到歌词
          <div class="text-xs mt-2 text-white/30">
            本地歌曲：到设置里打开「下载时同时下载歌词」，之后下的歌自带歌词
          </div>
        </div>
        <div
          v-for="(line, i) in lyricLines"
          :key="i"
          :data-active="i === lyricIndex ? '1' : ''"
          @click="onLineClick(line.time)"
          class="text-center py-1.5 cursor-pointer transition-all"
          :class="
            i === lyricIndex
              ? 'text-white font-bold text-base'
              : 'text-white/40 text-sm hover:text-white/70'
          "
        >
          {{ line.text || "···" }}
        </div>
        <div v-if="lyricLines.length && !isPlaying" class="text-center text-white/30 text-xs pt-4">
          已暂停
        </div>
      </div>
    </div>
  </div>
</template>
