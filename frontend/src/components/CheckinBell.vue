<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from "vue";
import { useI18n } from "vue-i18n";
import { useMainStore } from "../stores/main";
import OverlayMotion from "@/components/base/OverlayMotion.vue";
import { fetchCheckinStatus, openCheckinSite, type CheckinStatusItem } from "@/utils/checkin";

const store = useMainStore();
const { t } = useI18n();

const showPanel = ref(false);
const statusList = ref<CheckinStatusItem[]>([]);
const refreshing = ref(false);

const pendingCount = computed(() => statusList.value.filter((s) => !s.done).length);

const refresh = async () => {
  if (!store.isLogged) {
    statusList.value = [];
    return;
  }
  refreshing.value = true;
  try {
    statusList.value = await fetchCheckinStatus();
  } catch (e) {
    console.error("签到状态刷新失败", e);
  } finally {
    refreshing.value = false;
  }
};

const openSite = (item: CheckinStatusItem) => {
  openCheckinSite(item.site_id);
  const target = statusList.value.find((s) => s.site_id === item.site_id);
  if (target) target.done = true;
  setTimeout(() => void refresh(), 1500);
};

let timer: number | null = null;
onMounted(() => {
  void refresh();
  // 每 5 分钟刷新一次
  timer = window.setInterval(() => {
    void refresh();
  }, 5 * 60 * 1000);
});
onUnmounted(() => {
  if (timer) window.clearInterval(timer);
});

// 登录状态变化时重新拉取
watch(
  () => store.isLogged,
  () => void refresh(),
);
</script>

<template>
  <div class="relative">
    <button
      @click="showPanel = !showPanel"
      class="relative w-6 h-6 rounded-full bg-gray-100 text-gray-500 hover:bg-gray-200 flex items-center justify-center transition-all"
      :title="t('common.yunqi.bell.title')"
      :aria-label="t('common.yunqi.bell.title')"
    >
      <svg
        xmlns="http://www.w3.org/2000/svg"
        viewBox="0 0 24 24"
        fill="currentColor"
        class="w-4 h-4"
      >
        <path
          fill-rule="evenodd"
          d="M5.25 9a6.75 6.75 0 0113.5 0v.75c0 2.123.8 4.057 2.118 5.52a.75.75 0 01-.297 1.206c-1.544.57-3.16.99-4.831 1.243a3.75 3.75 0 11-7.48 0 24.585 24.585 0 01-4.831-1.244.75.75 0 01-.298-1.205A8.217 8.217 0 005.25 9.75V9zm4.502 8.9a2.25 2.25 0 104.496 0 25.057 25.057 0 01-4.496 0z"
          clip-rule="evenodd"
        />
      </svg>
      <span
        v-if="pendingCount > 0"
        class="absolute -top-1 -right-1 min-w-[16px] h-4 px-1 rounded-full bg-orange-500 text-white text-[10px] font-bold flex items-center justify-center"
      >
        {{ pendingCount > 99 ? "99+" : pendingCount }}
      </span>
    </button>

    <OverlayMotion
      :show="showPanel"
      :z-index="9999"
      variant="popover"
      panel-class="fixed p-4 bg-white rounded-2xl border border-gray-200 shadow-2xl text-gray-800 w-[320px] max-h-[70vh] overflow-y-auto"
      :panel-style="{ top: '64px', right: '16px' }"
    >
      <div class="flex items-center justify-between mb-2">
        <div class="text-sm font-bold text-gray-900">{{ t("common.yunqi.bell.today") }}</div>
        <div class="flex items-center gap-2">
          <button
            @click="refresh"
            :disabled="refreshing"
            class="text-xs text-gray-500 hover:text-gray-900 disabled:opacity-50"
          >
            {{ refreshing ? t("common.yunqi.checkin.refreshing") : t("common.yunqi.checkin.refresh") }}
          </button>
          <button
            @click="showPanel = false"
            class="text-gray-400 hover:text-gray-700 text-lg leading-none"
          >
            ×
          </button>
        </div>
      </div>
      <div v-if="!store.isLogged" class="text-xs text-gray-500 py-4 text-center">
        {{ t("common.yunqi.bell.loginFirst") }}
      </div>
      <div v-else-if="statusList.length === 0" class="text-xs text-gray-500 py-4 text-center">
        {{ t("common.yunqi.bell.noSites") }}
      </div>
      <div v-else class="space-y-1.5">
        <div
          v-for="item in statusList"
          :key="item.site_id"
          @click="openSite(item)"
          class="flex items-center gap-3 px-3 py-2.5 rounded-xl hover:bg-gray-100 cursor-pointer transition-colors border border-gray-100"
        >
          <div class="min-w-0 flex-1">
            <div class="text-sm font-medium text-gray-800 truncate">{{ item.name }}</div>
          </div>
          <span
            class="text-[11px] font-bold px-2 py-1 rounded-full shrink-0"
            :class="
              item.done
                ? 'bg-green-100 text-green-700'
                : 'bg-orange-100 text-orange-700'
            "
          >
            {{ item.done ? t("common.yunqi.checkin.signed") : t("common.yunqi.checkin.pending") }}
          </span>
        </div>
        <p class="text-[11px] text-gray-400 pt-1">{{ t("common.yunqi.checkin.tapHint") }}</p>
      </div>
    </OverlayMotion>
  </div>
</template>
