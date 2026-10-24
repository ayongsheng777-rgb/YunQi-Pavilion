<script setup lang="ts">
import { ref, computed, onMounted } from "vue";
import type { WidgetConfig } from "@/types";
import { useMainStore } from "../stores/main";
import {
  fetchHorizonList,
  fetchHorizonDay,
  renderHorizonMarkdown,
  type HorizonIndex,
} from "@/utils/horizon";

defineProps<{ widget: WidgetConfig }>();
const store = useMainStore();

const index = ref<HorizonIndex | null>(null);
const loading = ref(true);
const loadError = ref("");
const readerOpen = ref(false);
const readerLoading = ref(false);
const readerError = ref("");
const readerDate = ref("");
const readerHtml = ref("");
const dayCache = ref<Record<string, string>>({});

const hasReport = computed(() => !!index.value?.exists && (index.value?.dates?.length ?? 0) > 0);
const latestDate = computed(() => index.value?.latest ?? "");
const latestCount = computed(() => index.value?.counts?.[latestDate.value] ?? 0);
const dates = computed(() => index.value?.dates ?? []);

const reload = async () => {
  if (!store.isLogged) {
    loading.value = false;
    return;
  }
  loading.value = true;
  loadError.value = "";
  try {
    index.value = await fetchHorizonList();
  } catch (e) {
    loadError.value = e instanceof Error ? e.message : "加载失败";
  } finally {
    loading.value = false;
  }
};

const openReader = async (date?: string) => {
  const target = date || latestDate.value;
  if (!target) return;
  readerOpen.value = true;
  readerDate.value = target;
  if (dayCache.value[target]) {
    readerHtml.value = dayCache.value[target];
    return;
  }
  readerLoading.value = true;
  readerError.value = "";
  try {
    const day = await fetchHorizonDay(target);
    if (!day.exists || !day.markdown) {
      readerError.value = "这天没有日报";
      return;
    }
    const html = renderHorizonMarkdown(day.markdown);
    dayCache.value[target] = html;
    readerHtml.value = html;
  } catch (e) {
    readerError.value = e instanceof Error ? e.message : "加载失败";
  } finally {
    readerLoading.value = false;
  }
};

const closeReader = () => {
  readerOpen.value = false;
};

onMounted(() => {
  void reload();
});
</script>

<template>
  <div
    class="w-full h-full rounded-2xl backdrop-blur border border-white/10 overflow-hidden flex flex-col text-white relative transition-shadow"
    :style="{
      backgroundColor: `rgba(0,0,0,${Math.min(0.85, Math.max(0.15, widget.opacity ?? 0.35))})`,
      color: '#fff',
    }"
  >
    <div class="px-4 py-3 border-b border-white/10 flex items-center justify-between bg-white/10 shrink-0">
      <div class="flex items-center gap-2">
        <span class="text-lg">🛰️</span>
        <span class="font-bold text-sm text-white">Horizon 日报</span>
      </div>
      <button
        class="text-xs bg-white/10 text-white/70 px-2 py-0.5 rounded hover:bg-white/20 hover:text-white"
        @click="reload"
        title="刷新"
      >
        ↻
      </button>
    </div>

    <div class="flex-1 min-h-0 p-4 flex flex-col">
      <div v-if="loading" class="flex-1 flex items-center justify-center text-xs text-white/60 text-center">加载中…</div>
      <div v-else-if="loadError" class="flex-1 flex items-center justify-center text-xs text-red-300 text-center">{{ loadError }}</div>
      <div v-else-if="!hasReport" class="flex-1 flex items-center justify-center text-xs text-white/60 text-center">
        暂无日报<br />等待 OECT 每天 8 点推送
      </div>
      <div v-else class="flex-1 min-h-0 overflow-hidden flex flex-col text-xs">
        <div class="text-white/60 mb-2">{{ latestDate }} · 共 {{ latestCount }} 条</div>
        <button
          class="mb-2 text-left text-sky-300 hover:text-sky-200 hover:underline shrink-0"
          @click="openReader()"
        >
          看全文 →
        </button>
        <div class="text-white/45">往期：</div>
        <div class="flex flex-wrap gap-1 mt-1">
          <button
            v-for="d in dates.slice(0, 7)"
            :key="d"
            class="bg-white/10 text-white/75 px-2 py-0.5 rounded hover:bg-white/20 hover:text-white"
            @click="openReader(d)"
          >
            {{ d.slice(5) }}
          </button>
        </div>
      </div>
    </div>

    <!-- 全文阅读页 -->
    <div
      v-if="readerOpen"
      class="fixed inset-0 z-[100] bg-black/80 flex items-center justify-center p-4"
      @click.self="closeReader"
    >
      <div class="w-full max-w-3xl max-h-[90vh] rounded-2xl bg-[#14181f] border border-white/10 flex flex-col overflow-hidden">
        <div class="px-5 py-3 border-b border-white/10 flex items-center justify-between shrink-0">
          <div class="font-bold text-sm text-white">Horizon 日报 · {{ readerDate }}</div>
          <div class="flex items-center gap-2">
            <button
              v-for="d in dates.slice(0, 7)"
              :key="d"
              class="text-xs px-2 py-0.5 rounded"
              :class="d === readerDate ? 'bg-sky-500 text-white' : 'bg-white/10 text-white/70 hover:bg-white/20'"
              @click="openReader(d)"
            >
              {{ d.slice(5) }}
            </button>
            <button class="text-white/70 hover:text-white text-lg leading-none px-1" @click="closeReader" title="关闭">×</button>
          </div>
        </div>
        <div class="flex-1 overflow-y-auto px-5 py-4 text-sm text-white/85 horizon-reader">
          <div v-if="readerLoading" class="text-center text-white/60 py-8">加载中…</div>
          <div v-else-if="readerError" class="text-center text-red-300 py-8">{{ readerError }}</div>
          <div v-else v-html="readerHtml"></div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.horizon-reader :deep(h1) {
  font-size: 1.25rem;
  font-weight: 700;
  margin: 0.5rem 0 0.75rem;
  color: #fff;
}
.horizon-reader :deep(h2) {
  font-size: 1.1rem;
  font-weight: 700;
  margin: 1rem 0 0.5rem;
  color: #fff;
}
.horizon-reader :deep(h3) {
  font-size: 0.95rem;
  font-weight: 600;
  margin: 0.75rem 0 0.25rem;
  color: #7dd3fc;
}
.horizon-reader :deep(h3) a {
  color: #7dd3fc;
}
.horizon-reader :deep(p) {
  margin: 0.25rem 0 0.5rem;
  line-height: 1.7;
}
.horizon-reader :deep(a) {
  color: #7dd3fc;
}
.horizon-reader :deep(a):hover {
  text-decoration: underline;
}
.horizon-reader :deep(hr) {
  border-color: rgba(255, 255, 255, 0.1);
  margin: 0.75rem 0;
}
.horizon-reader :deep(blockquote) {
  border-left: 3px solid rgba(125, 211, 252, 0.5);
  padding-left: 0.75rem;
  color: rgba(255, 255, 255, 0.6);
  margin: 0.5rem 0;
}
.horizon-reader :deep(ul) {
  list-style: disc;
  padding-left: 1.25rem;
  margin: 0.25rem 0 0.5rem;
}
.horizon-reader :deep(code) {
  background: rgba(255, 255, 255, 0.1);
  padding: 0 0.25rem;
  border-radius: 0.25rem;
}
</style>
