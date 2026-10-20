<script setup lang="ts">
import { ref, onMounted, watch, nextTick } from "vue";
import { useMusicPlayer } from "@/stores/musicPlayer";
import MusicPanel from "@/components/MusicPanel.vue";

const player = useMusicPlayer();
const { current, isPlaying, resolving, currentLyric, playError } = player;

// ---------- 跑马灯（沿用原播放器的做法） ----------
const titleWrapRef = ref<HTMLElement | null>(null);
const titleTextRef = ref<HTMLElement | null>(null);
const marqueeEnabled = ref(false);
const marqueeDuration = ref("3s");
const marqueeTo = ref("0px");

const setupMarquee = () => {
  const wrap = titleWrapRef.value;
  const text = titleTextRef.value;
  if (!wrap || !text) {
    marqueeEnabled.value = false;
    return;
  }
  const overflow = text.scrollWidth - wrap.clientWidth;
  if (overflow > 0 && isPlaying.value) {
    marqueeEnabled.value = true;
    marqueeDuration.value = Math.max(overflow / 50, 2) + "s";
    marqueeTo.value = "-" + overflow + "px";
  } else {
    marqueeEnabled.value = false;
  }
};

watch([() => current.value?.name, isPlaying], async () => {
  await nextTick();
  setupMarquee();
});

const statusText = () => {
  if (resolving.value) return "正在解析…";
  if (playError.value) return playError.value;
  if (currentLyric.value) return currentLyric.value;
  if (!current.value) return "点右侧图标选歌";
  return isPlaying.value ? "正在播放" : "已暂停";
};

onMounted(() => {
  void player.initPlayer();
});
</script>

<template>
  <div
    class="h-10 w-10 xl:w-[230px] px-0 rounded-full bg-transparent flex items-center gap-2 transition-all group select-none relative left-0 xl:-left-[5px]"
  >
    <button
      @click="player.togglePlay"
      class="w-10 h-10 flex-shrink-0 rounded-full bg-white/20 flex items-center justify-center hover:bg-white/40 hover:text-white transition-all active:scale-95 text-white backdrop-blur border border-white/20 shadow-sm"
      :title="current ? (isPlaying ? '暂停' : '播放') : '点歌'"
    >
      <svg
        v-if="isPlaying"
        xmlns="http://www.w3.org/2000/svg"
        viewBox="0 0 24 24"
        fill="currentColor"
        class="w-6 h-6"
      >
        <path
          fill-rule="evenodd"
          d="M6.75 5.25a.75.75 0 01.75-.75H9a.75.75 0 01.75.75v13.5a.75.75 0 01-.75.75H7.5a.75.75 0 01-.75-.75V5.25zm7.5 0A.75.75 0 0115 4.5h1.5a.75.75 0 01.75.75v13.5a.75.75 0 01.75.75H15a.75.75 0 01-.75-.75V5.25z"
          clip-rule="evenodd"
        />
      </svg>
      <svg
        v-else
        xmlns="http://www.w3.org/2000/svg"
        viewBox="0 0 24 24"
        fill="currentColor"
        class="w-6 h-6 pl-0.5"
      >
        <path
          fill-rule="evenodd"
          d="M4.5 5.653c0-1.426 1.529-2.33 2.779-1.643l11.54 6.348c1.295.712 1.295 2.573 0 3.285L7.28 19.991c-1.25.687-2.779-.217-2.779-1.643V5.653z"
          clip-rule="evenodd"
        />
      </svg>
    </button>

    <div
      ref="titleWrapRef"
      class="flex-1 overflow-hidden text-xs font-medium text-white text-shadow hidden xl:flex flex-col justify-center min-w-0 h-full py-0.5"
      :title="current ? current.name + ' - ' + current.singer : '云栖阁音乐'"
    >
      <span
        ref="titleTextRef"
        :style="
          marqueeEnabled
            ? {
                '--marquee-to': marqueeTo,
                animation: `marqueeX ${marqueeDuration} linear infinite alternate`,
                display: 'inline-block',
                whiteSpace: 'nowrap',
              }
            : { whiteSpace: 'nowrap' }
        "
        >{{ current ? current.name : "云栖阁音乐" }}</span
      >
      <div class="flex items-center w-full pr-0">
        <span class="text-[12px] opacity-70 origin-left truncate w-auto mr-auto">{{
          statusText()
        }}</span>
        <div class="flex items-center gap-0.5 flex-shrink-0 origin-right">
          <button
            @click="player.step(-1)"
            class="w-6 h-6 rounded-full flex items-center justify-center text-white/80 hover:text-white hover:bg-white/20 transition-all disabled:opacity-30 disabled:hover:bg-transparent disabled:cursor-not-allowed"
            title="上一首"
            :disabled="!player.queue.value.length"
          >
            <svg
              xmlns="http://www.w3.org/2000/svg"
              viewBox="0 0 24 24"
              fill="currentColor"
              class="w-4 h-4"
            >
              <path
                d="M9.195 18.44c1.25.713 2.805-.19 2.805-1.629v-2.81l7.82 4.466c1.25.713 2.805-.19 2.805-1.629V7.162c0-1.44-1.555-2.342-2.805-1.628l-7.82 4.466v-2.81c0-1.44-1.555-2.343-2.805-1.629l-7.108 4.062c-1.26.72-1.26 2.536 0 3.256l7.108 4.061z"
              />
            </svg>
          </button>
          <button
            @click="player.step(1)"
            class="w-6 h-6 rounded-full flex items-center justify-center text-white/80 hover:text-white hover:bg-white/20 transition-all disabled:opacity-30 disabled:hover:bg-transparent"
            title="下一首"
            :disabled="!player.queue.value.length"
          >
            <svg
              xmlns="http://www.w3.org/2000/svg"
              viewBox="0 0 24 24"
              fill="currentColor"
              class="w-4 h-4"
            >
              <path
                d="M5.055 7.06C3.805 6.347 2.25 7.25 2.25 8.69v8.122c0 1.44 1.555 2.342 2.805 1.628L12.875 14v2.81c0 1.44 1.555 2.343 2.805 1.629l7.108-4.061c1.26-.72 1.26-2.536 0-3.256l-7.108-4.062c-1.25-.713-2.805.19-2.805 1.629V11.5L5.055 7.06z"
              />
            </svg>
          </button>
          <button
            @click="player.openPanel('search')"
            class="w-6 h-6 rounded-full flex items-center justify-center text-white/80 hover:text-white hover:bg-white/20 transition-all"
            title="点歌"
          >
            <svg
              xmlns="http://www.w3.org/2000/svg"
              viewBox="0 0 24 24"
              fill="currentColor"
              class="w-4 h-4"
            >
              <path
                fill-rule="evenodd"
                d="M9.965 11.026a5 5 0 118.06 4.06l3.288 3.287a.75.75 0 11-1.06 1.06l-3.288-3.287a5 5 0 01-6.928 0 5 5 0 010-7.12zM10.965 5a4 4 0 100 8 4 4 0 000-8z"
                clip-rule="evenodd"
              />
            </svg>
          </button>
        </div>
      </div>
    </div>

    <MusicPanel />
  </div>
</template>

<style scoped>
.text-shadow {
  text-shadow: 0 1px 2px rgba(0, 0, 0, 0.3);
}
@keyframes marqueeX {
  0% {
    transform: translateX(0);
  }
  100% {
    transform: translateX(var(--marquee-to));
  }
}
</style>
