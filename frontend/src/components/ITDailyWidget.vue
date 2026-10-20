<script setup lang="ts">
import { ref, computed, onMounted} from "vue";
import type { WidgetConfig} from "@/types";
import { useMainStore} from "../stores/main";
import { fetchITDailyLatest, type ITDailyReport} from "@/utils/itdaily";

defineProps<{ widget: WidgetConfig}>();
const store = useMainStore();

const report = ref<ITDailyReport | null>(null);
const loading = ref(true);
const loadError = ref("");
const expanded = ref(false);

const hasReport = computed(() =>!!report.value?.exists);
const sections = computed(() => report.value?.sections?? []);
const totalItems = computed(() => report.value?.count?? sections.value.reduce((n, s) => n + (s.items?.length?? 0), 0));

const reload = async () => {
if (!store.isLogged) {
loading.value = false;
return;
}
loading.value = true;
loadError.value = "";
try {
report.value = await fetchITDailyLatest();
} catch (e) {
loadError.value = e instanceof Error? e.message: "加载失败";
} finally {
loading.value = false;
}
};

onMounted(() => {
void reload();
});
</script>

<template>
<div class="p-4 h-full flex flex-col">
<div class="flex items-center justify-between mb-2">
<div class="flex items-center gap-2">
<span class="text-lg">📰</span>
<span class="font-semibold text-sm">IT&AI 日报</span>
</div>
<button
class="text-xs text-gray-400 hover:text-gray-600"
@click="reload"
title="刷新"
>
↻
</button>
</div>

<div v-if="loading" class="text-xs text-gray-400 py-4 text-center">加载中…</div>
<div v-else-if="loadError" class="text-xs text-red-400 py-4 text-center">{{ loadError}}</div>
<div v-else-if="!hasReport" class="text-xs text-gray-400 py-4 text-center">
暂无日报<br />等待 n8n 每日 19 点推送
</div>
<div v-else class="flex-1 overflow-hidden flex flex-col text-xs">
<div class="text-gray-500 mb-2">
{{ report?.reportDate}} · 共 {{ totalItems}} 条
</div>

<div v-if="!expanded" class="flex-1 overflow-y-auto space-y-2">
<div v-for="sec in sections":key="sec.name" class="border-b border-gray-100 pb-1">
<div class="font-medium text-gray-700 mb-1">{{ sec.name}}（{{ sec.items?.length?? 0}}）</div>
<a
v-for="item in (sec.items?? []).slice(0, 3)"
:key="item.idx"
:href="item.url"
target="_blank"
rel="noopener"
class="block truncate text-gray-600 hover:text-blue-600 py-0.5"
:title="item.brief || item.title"
>
{{ item.idx}}. {{ item.title}}
</a>
<div v-if="(sec.items?.length?? 0) > 3" class="text-gray-400">…等 {{ (sec.items?.length?? 0) - 3}} 条</div>
</div>
</div>

<div v-if="expanded" class="flex-1 overflow-y-auto space-y-3">
<div v-for="sec in sections":key="sec.name">
<div class="font-medium text-gray-700 mb-1"></div>
<div v-for="item in sec.items?? []":key="item.idx" class="mb-2 pl-1">
<a:href="item.url" target="_blank" rel="noopener" class="text-blue-600 hover:underline">
{{ item.idx}}. {{ item.title}}
</a>
<div v-if="item.brief" class="text-gray-600 mt-0.5">{{ item.brief}}</div>
<div v-if="item.impact" class="text-gray-500 mt-0.5">影响：{{ item.impact}}</div>
</div>
</div>
<div v-if="report?.trend">
<div class="font-medium text-gray-700 mb-1"></div>
<div class="text-gray-600 leading-relaxed">{{ report.trend}}</div>
</div>
<div v-if="(report?.bugs?.length?? 0) > 0">
<div class="font-medium text-gray-700 mb-1">{{ report?.bugs?.length}} 条</div>
<div v-for="b in report?.bugs?? []":key="b.title" class="mb-1.5 pl-1">
<a:href="b.url" target="_blank" rel="noopener" class="text-blue-600 hover:underline">{{ b.title}}</a>
<div v-if="b.problem" class="text-gray-600">问题：{{ b.problem}}</div>
<div v-if="b.solution" class="text-gray-500">对策：{{ b.solution}}</div>
</div>
</div>
</div>

<button
class="mt-2 text-center text-blue-500 hover:text-blue-700 text-xs py-1 border-t border-gray-100"
@click="expanded =!expanded"
>
{{ expanded? "收起 ▲": "展开看全文 ▼"}}
</button>
</div>
</div>
</template>
