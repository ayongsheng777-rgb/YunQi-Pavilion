<script setup lang="ts">
import { ref, computed, onMounted } from "vue";
import { useI18n } from "vue-i18n";
import type { WidgetConfig } from "@/types";
import { useMainStore } from "../stores/main";
import OverlayMotion from "@/components/base/OverlayMotion.vue";
import {
  fetchCheckinSites,
  fetchCheckinStatus,
  addCheckinSite,
  updateCheckinSite,
  deleteCheckinSite,
  openCheckinSite,
  searchCheckinBookmarks,
  type CheckinSite,
  type CheckinStatusItem,
  type BookmarkMatch,
} from "@/utils/checkin";

defineProps<{ widget: WidgetConfig }>();
const { t } = useI18n();
const store = useMainStore();

const sites = ref<CheckinSite[]>([]);
const statusList = ref<CheckinStatusItem[]>([]);
const loading = ref(true);
const loadError = ref("");
const refreshing = ref(false);

const doneMap = computed(() => {
  const m: Record<string, boolean> = {};
  for (const s of statusList.value) m[s.site_id] = !!s.done;
  return m;
});
const enabledSites = computed(() => sites.value.filter((s) => s.enabled));
const pendingCount = computed(() => enabledSites.value.filter((s) => !doneMap.value[s.id]).length);

const reload = async (silent = false) => {
  if (!store.isLogged) {
    sites.value = [];
    statusList.value = [];
    loading.value = false;
    return;
  }
  if (!silent) loading.value = true;
  refreshing.value = true;
  loadError.value = "";
  try {
    const [s, st] = await Promise.all([fetchCheckinSites(), fetchCheckinStatus()]);
    sites.value = s;
    statusList.value = st;
  } catch (e) {
    loadError.value = e instanceof Error ? e.message : t("common.yunqi.checkin.loading");
  } finally {
    loading.value = false;
    refreshing.value = false;
  }
};

onMounted(() => {
  void reload();
});

const openSite = (site: CheckinSite) => {
  openCheckinSite(site.id);
  // 乐观更新徽章：点开就视为已签到，后台会以真实状态为准
  const item = statusList.value.find((s) => s.site_id === site.id);
  if (item) item.done = true;
  else statusList.value.push({ site_id: site.id, name: site.name, url: site.url, done: true });
  // 顺便把状态拉一遍，保证准确
  setTimeout(() => void reload(true), 1500);
};

// ---------- 管理站点 ----------
const showManage = ref(false);
const editing = ref<CheckinSite | null>(null);
const form = ref({ name: "", url: "", icon: "", note: "", enabled: true });

// --- 书签自动匹配：输入站点名/网址时，检索已收藏书签供一键填充 ---
const matches = ref<BookmarkMatch[]>([]);
const showMatches = ref(false);
let matchTimer: ReturnType<typeof setTimeout> | null = null;
const onFormInput = () => {
  if (matchTimer) clearTimeout(matchTimer);
  const q = (form.value.name || form.value.url || "").trim();
  if (!q) {
    matches.value = [];
    showMatches.value = false;
    return;
  }
  matchTimer = setTimeout(async () => {
    try {
      matches.value = await searchCheckinBookmarks(q);
      showMatches.value = matches.value.length > 0;
    } catch {
      matches.value = [];
      showMatches.value = false;
    }
  }, 300);
};
const applyMatch = (m: BookmarkMatch) => {
  form.value.name = m.title;
  form.value.url = m.url;
  if (m.icon) form.value.icon = m.icon;
  showMatches.value = false;
};
const saving = ref(false);

const openManage = () => {
  showManage.value = true;
};

const startAdd = () => {
  editing.value = null;
  form.value = { name: "", url: "", icon: "", note: "", enabled: true };
};

const startEdit = (site: CheckinSite) => {
  editing.value = site;
  form.value = {
    name: site.name,
    url: site.url,
    icon: site.icon || "",
    note: site.note || "",
    enabled: site.enabled !== false,
  };
};

const cancelEdit = () => {
  editing.value = null;
  form.value = { name: "", url: "", icon: "", note: "", enabled: true };
};

const normalizeUrl = (u: string) => {
  const t2 = u.trim();
  if (!t2) return t2;
  return /^https?:\/\//i.test(t2) ? t2 : "https://" + t2;
};

const confirmSave = async () => {
  if (!form.value.name.trim()) {
    alert(t("common.yunqi.checkin.needName"));
    return;
  }
  if (!form.value.url.trim()) {
    alert(t("common.yunqi.checkin.needUrl"));
    return;
  }
  saving.value = true;
  try {
    const payload = {
      name: form.value.name.trim(),
      url: normalizeUrl(form.value.url),
      icon: form.value.icon.trim(),
      note: form.value.note.trim(),
      enabled: form.value.enabled,
    };
    if (editing.value) {
      await updateCheckinSite({ ...editing.value, ...payload });
    } else {
      await addCheckinSite(payload);
    }
    cancelEdit();
    await reload(true);
  } catch (e) {
    alert(e instanceof Error ? e.message : t("common.yunqi.checkin.saving"));
  } finally {
    saving.value = false;
  }
};

const removeSite = async (site: CheckinSite) => {
  if (!confirm(t("common.yunqi.checkin.confirmDelete", { name: site.name }))) return;
  try {
    await deleteCheckinSite(site.id);
    await reload(true);
  } catch (e) {
    alert(e instanceof Error ? e.message : t("common.yunqi.checkin.delete"));
  }
};

const toggleEnabled = async (site: CheckinSite) => {
  try {
    await updateCheckinSite({ ...site, enabled: !site.enabled });
    site.enabled = !site.enabled;
  } catch (e) {
    alert(e instanceof Error ? e.message : t("common.yunqi.checkin.saving"));
  }
};
</script>

<template>
  <div
    class="w-full h-full rounded-2xl backdrop-blur border border-white/10 overflow-hidden flex flex-col text-white relative group"
    :style="{
      backgroundColor: `rgba(0,0,0,${Math.min(0.85, Math.max(0.15, widget.opacity ?? 0.35))})`,
      color: '#fff',
    }"
  >
    <div
      class="px-4 py-3 border-b border-white/10 flex justify-between items-center bg-white/10 shrink-0"
    >
      <div class="font-bold text-sm flex items-center gap-2 text-white">
        ✅ {{ t("common.yunqi.checkin.title") }}
        <span
          v-if="pendingCount > 0"
          class="text-[10px] px-2 py-0.5 rounded-full bg-orange-500/80 text-white font-bold"
          >{{ t("common.yunqi.checkin.pendingCount", { count: pendingCount }) }}</span
        >
      </div>
      <div class="flex items-center gap-1">
        <button
          v-if="store.isLogged"
          @click="openManage"
          class="text-xs bg-white/10 text-white/70 px-2 py-0.5 rounded hover:bg-white/20"
        >
          {{ t("common.yunqi.checkin.manage") }}
        </button>
        <button
          v-if="store.isLogged"
          @click="reload()"
          :disabled="refreshing"
          class="text-xs bg-white/10 text-white/70 px-2 py-0.5 rounded hover:bg-white/20 disabled:opacity-50"
          :title="t('common.yunqi.checkin.refresh')"
        >
          {{ refreshing ? t("common.yunqi.checkin.refreshing") : t("common.yunqi.checkin.refresh") }}
        </button>
      </div>
    </div>

    <div class="flex-1 overflow-y-auto p-3 space-y-1.5 scrollbar-hide">
      <div v-if="!store.isLogged" class="text-xs text-white/60 text-center py-6">
        {{ t("common.yunqi.checkin.loginFirst") }}
      </div>
      <div v-else-if="loading" class="text-xs text-white/60 text-center py-6">
        {{ t("common.yunqi.checkin.loading") }}
      </div>
      <div v-else-if="loadError" class="text-xs text-red-300 text-center py-6">
        {{ loadError }}
        <button @click="reload()" class="underline ml-2 text-white/80">
          {{ t("common.yunqi.checkin.retry") }}
        </button>
      </div>
      <div v-else-if="enabledSites.length === 0" class="text-xs text-white/60 text-center py-6">
        {{ t("common.yunqi.checkin.noSites") }}
      </div>
      <div
        v-for="site in enabledSites"
        :key="site.id"
        @click="openSite(site)"
        class="flex items-center gap-3 px-3 py-2.5 rounded-xl bg-white/5 hover:bg-white/15 border border-white/10 cursor-pointer transition-all"
        :title="site.note || site.url"
      >
        <div
          class="w-8 h-8 rounded-lg bg-white/10 flex items-center justify-center shrink-0 overflow-hidden border border-white/10 text-lg"
        >
          <img
            v-if="site.icon"
            :src="site.icon"
            class="w-6 h-6 object-contain"
            @error="site.icon = ''"
            alt=""
          />
          <span v-else>🌐</span>
        </div>
        <div class="min-w-0 flex-1">
          <div class="text-sm font-medium text-white/90 truncate">{{ site.name }}</div>
          <div v-if="site.note" class="text-[11px] text-white/50 truncate">{{ site.note }}</div>
        </div>
        <span
          class="text-[11px] font-bold px-2 py-1 rounded-full shrink-0"
          :class="
            doneMap[site.id]
              ? 'bg-green-500/25 text-green-300 border border-green-400/30'
              : 'bg-orange-500/25 text-orange-300 border border-orange-400/30'
          "
        >
          {{ doneMap[site.id] ? t("common.yunqi.checkin.signed") : t("common.yunqi.checkin.pending") }}
        </span>
      </div>
    </div>
  </div>

  <OverlayMotion
    :show="showManage"
    :z-index="9999"
    variant="dialog"
    panel-class="fixed left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 p-5 bg-neutral-900/95 backdrop-blur rounded-2xl border border-white/10 shadow-2xl text-white w-[min(560px,92vw)] max-h-[85vh] overflow-y-auto"
  >
    <div class="flex items-center justify-between mb-3">
      <div class="text-sm font-bold">{{ t("common.yunqi.checkin.manageTitle") }}</div>
      <button @click="showManage = false" class="text-white/60 hover:text-white text-lg leading-none">×</button>
    </div>

    <!-- 站点列表 -->
    <div class="space-y-2 mb-4">
      <div
        v-for="site in sites"
        :key="site.id"
        class="flex items-center gap-2 px-3 py-2 rounded-xl bg-white/5 border border-white/10"
      >
        <div class="min-w-0 flex-1">
          <div class="text-sm font-medium truncate">
            {{ site.name }}
            <span v-if="!site.enabled" class="text-[10px] text-white/40 ml-1">{{
              t("common.yunqi.checkin.disabled")
            }}</span>
          </div>
          <div class="text-[11px] text-white/50 truncate">{{ site.url }}</div>
        </div>
        <button
          @click="toggleEnabled(site)"
          class="text-[11px] px-2 py-1 rounded bg-white/10 hover:bg-white/20 text-white/80"
          :title="site.enabled ? t('common.yunqi.checkin.disable') : t('common.yunqi.checkin.enable')"
        >
          {{ site.enabled ? t("common.yunqi.checkin.disable") : t("common.yunqi.checkin.enable") }}
        </button>
        <button
          @click="startEdit(site)"
          class="text-[11px] px-2 py-1 rounded bg-white/10 hover:bg-white/20 text-white/80"
        >
          {{ t("common.yunqi.checkin.edit") }}
        </button>
        <button
          @click="removeSite(site)"
          class="text-[11px] px-2 py-1 rounded bg-red-500/20 hover:bg-red-500/40 text-red-300"
        >
          {{ t("common.yunqi.checkin.delete") }}
        </button>
      </div>
      <div v-if="sites.length === 0" class="text-xs text-white/50 text-center py-3">
        {{ t("common.yunqi.checkin.emptyList") }}
      </div>
    </div>

    <!-- 新增 / 编辑表单 -->
    <div class="border-t border-white/10 pt-3">
      <div class="text-xs font-bold text-white/70 mb-2">
        {{ editing ? t("common.yunqi.checkin.editSite") : t("common.yunqi.checkin.addSite") }}
      </div>
      <div class="grid grid-cols-1 gap-2 relative">
        <input
          v-model="form.name"
          @input="onFormInput"
          @focus="onFormInput"
          :placeholder="t('common.yunqi.checkin.namePh')"
          class="text-sm px-3 py-2 rounded-lg border bg-white/10 text-white placeholder-white/40 focus:outline-none focus:border-white/40"
        />
        <input
          v-model="form.url"
          @input="onFormInput"
          @focus="onFormInput"
          :placeholder="t('common.yunqi.checkin.urlPh')"
          class="text-sm px-3 py-2 rounded-lg border bg-white/10 text-white placeholder-white/40 focus:outline-none focus:border-white/40"
        />
        <div
          v-if="showMatches"
          class="absolute left-0 right-0 top-full mt-1 z-20 rounded-lg bg-gray-900 border border-white/20 shadow-xl overflow-hidden"
        >
          <div class="px-3 py-1.5 text-[11px] text-white/50 border-b border-white/10">已收藏的书签（点一下自动填入）</div>
          <button
            v-for="m in matches"
            :key="m.id"
            @click="applyMatch(m)"
            class="w-full text-left px-3 py-2 hover:bg-white/10 flex items-center gap-2"
          >
            <span class="flex-1 min-w-0">
              <span class="block text-sm text-white truncate">{{ m.title }}</span>
              <span class="block text-[11px] text-white/50 truncate">{{ m.url }}</span>
            </span>
            <span class="text-[11px] text-white/40 whitespace-nowrap">{{ m.category }}</span>
          </button>
        </div>
        <input
          v-model="form.icon"
          :placeholder="t('common.yunqi.checkin.iconPh')"
          class="text-sm px-3 py-2 rounded-lg border bg-white/10 text-white placeholder-white/40 focus:outline-none focus:border-white/40"
        />
        <input
          v-model="form.note"
          :placeholder="t('common.yunqi.checkin.notePh')"
          class="text-sm px-3 py-2 rounded-lg border bg-white/10 text-white placeholder-white/40 focus:outline-none focus:border-white/40"
        />
        <label class="flex items-center gap-2 text-xs text-white/70 cursor-pointer select-none">
          <input type="checkbox" v-model="form.enabled" class="w-4 h-4 accent-orange-500" />
          {{ t("common.yunqi.checkin.enableHint") }}
        </label>
      </div>
      <div class="flex justify-end gap-2 mt-3">
        <button
          v-if="editing"
          @click="cancelEdit"
          class="text-xs text-white/70 hover:bg-white/10 px-3 py-2 rounded-lg transition-colors"
        >
          {{ t("common.yunqi.checkin.cancelEdit") }}
        </button>
        <button
          v-if="editing"
          @click="startAdd"
          class="text-xs text-white/70 hover:bg-white/10 px-3 py-2 rounded-lg transition-colors"
        >
          {{ t("common.yunqi.checkin.switchToAdd") }}
        </button>
        <button
          @click="confirmSave"
          :disabled="saving"
          class="text-xs bg-orange-500/80 text-white px-4 py-2 rounded-lg hover:bg-orange-500 font-bold disabled:opacity-50"
        >
          {{
            saving
              ? t("common.yunqi.checkin.saving")
              : editing
                ? t("common.yunqi.checkin.saveChanges")
                : t("common.yunqi.checkin.add")
          }}
        </button>
      </div>
    </div>
  </OverlayMotion>
</template>

<style scoped>
.scrollbar-hide::-webkit-scrollbar {
  display: none;
}
</style>
