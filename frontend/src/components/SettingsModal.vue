<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed, watch } from "vue";
import { useStorage } from "@vueuse/core";
import { useI18n } from "vue-i18n";
import { useMainStore } from "../stores/main";
import { useI18nStore, type SupportedLocale } from "@/stores/i18n";
import type { WidgetConfig, NavGroup, NavItem } from "@/types";
import IconUploader from "./IconUploader.vue";
import WallpaperLibrary from "./WallpaperLibrary.vue";
import PasswordConfirmModal from "./PasswordConfirmModal.vue";
import DockerWidget from "./DockerWidget.vue";
import ProxyToggle from "./ProxyToggle.vue";
import SystemStatusWidget from "./SystemStatusWidget.vue";
import RssSettings from "./RssSettings.vue";
import SearchSettings from "./SearchSettings.vue";
import ScriptManager from "./ScriptManager.vue";
import OverlayMotion from "@/components/base/OverlayMotion.vue";
import QrcodeVue from "qrcode.vue";
import { DEFAULT_NETWORK_RULES, NETWORK_PRESET_RULES } from "@/utils/network";
import { createDefaultWidgetList } from "@/utils/widgetUtils";
import { toApiUrl } from "@/utils/runtimeUrls";
import daoliyuLogo from "@/assets/daoliyu.svg";

const { t } = useI18n();
const props = defineProps<{ show: boolean }>();
const emit = defineEmits(["update:show"]);
const store = useMainStore();
const i18nStore = useI18nStore();

const LOCALE_LABELS: Record<SupportedLocale, string> = {
  "zh-CN": "简体中文",
  "en-US": "English",
  "ja-JP": "日本語",
  "de-DE": "Deutsch",
  "zh-TW": "繁體中文",
  "fr-FR": "Français",
  "es-ES": "Español",
};

const LOCALE_FLAGS: Record<SupportedLocale, string> = {
  "zh-CN": "🇨🇳",
  "en-US": "🇺🇸",
  "ja-JP": "🇯🇵",
  "de-DE": "🇩🇪",
  "zh-TW": "🇹🇼",
  "fr-FR": "🇫🇷",
  "es-ES": "🇪🇸",
};

// 自动保存所有设置修改
watch(
  [() => store.widgets, () => store.appConfig],
  () => {
    if (props.show) {
      store.saveData();
    }
  },
  { deep: true }
);

const handleEscapeKey = (e: KeyboardEvent) => {
  if (e.key === "Escape" && props.show) {
    e.stopPropagation();
    emit("update:show", false);
  }
};

onMounted(() => {
  store.lockServerSync();
  document.addEventListener("keydown", handleEscapeKey);
});
onUnmounted(() => {
  store.unlockServerSync();
  document.removeEventListener("keydown", handleEscapeKey);
});

const DEFAULT_LATENCY_THRESHOLD_MS = 200;
const latencyThresholdDraft = ref("");
const latencyThresholdTouched = ref(false);
const latencyThresholdAppliedToast = ref("");
let latencyThresholdToastTimer: number | null = null;
const latencyThresholdValidation = computed(() => {
  const raw = latencyThresholdDraft.value.trim();
  if (!raw) return { ok: false, value: null as number | null, error: t('settings.validation.latencyInput') };
  if (!/^\d+$/.test(raw)) return { ok: false, value: null as number | null, error: t('settings.validation.positiveInteger') };
  const n = Number.parseInt(raw, 10);
  if (!Number.isFinite(n)) return { ok: false, value: null as number | null, error: t('settings.validation.invalidValue') };
  if (n < 20 || n > 30000) return { ok: false, value: n, error: t('settings.validation.latencyRange') };
  return { ok: true, value: n, error: "" };
});
const whitelistLatencyEnabled = computed(() => {
  return store.appConfig.whitelistLatencyMode === true;
});
const toggleWhitelistLatency = () => {
  store.appConfig.whitelistLatencyMode = !store.appConfig.whitelistLatencyMode;
  store.markDirty();
};
const syncLatencyThresholdDraft = () => {
  const v = store.appConfig.latencyThresholdMs ?? DEFAULT_LATENCY_THRESHOLD_MS;
  latencyThresholdDraft.value = String(v);
  latencyThresholdTouched.value = false;
};
watch(
  () => store.forceNetworkMode,
  (mode) => {
    if (mode === "latency") syncLatencyThresholdDraft();
  },
  { immediate: true },
);
watch(
  () => store.appConfig.latencyThresholdMs,
  () => {
    if (!latencyThresholdTouched.value) syncLatencyThresholdDraft();
  },
);
const onLatencyThresholdInput = (e: Event) => {
  latencyThresholdTouched.value = true;
  const raw = (e.target as HTMLInputElement).value ?? "";
  const digits = raw.replace(/[^\d]/g, "");
  latencyThresholdDraft.value = digits;
};
const applyLatencyThreshold = async () => {
  const v = latencyThresholdValidation.value;
  if (!v.ok || typeof v.value !== "number") return;
  store.appConfig.latencyThresholdMs = v.value;
  latencyThresholdTouched.value = false;
  store.markDirty();
  latencyThresholdAppliedToast.value = `${t('settings.messages.saved')}：${v.value} ms`;
  if (latencyThresholdToastTimer) window.clearTimeout(latencyThresholdToastTimer);
  latencyThresholdToastTimer = window.setTimeout(() => {
    latencyThresholdAppliedToast.value = "";
    latencyThresholdToastTimer = null;
  }, 1200);
};
const onLatencyThresholdBlur = async () => {
  if (!latencyThresholdTouched.value) return;
  const v = latencyThresholdValidation.value;
  if (!v.ok || typeof v.value !== "number") {
    syncLatencyThresholdDraft();
    return;
  }
  await applyLatencyThreshold();
};
const resetLatencyThreshold = async () => {
  store.appConfig.latencyThresholdMs = DEFAULT_LATENCY_THRESHOLD_MS;
  syncLatencyThresholdDraft();
  store.markDirty();
  latencyThresholdAppliedToast.value = `${t('settings.messages.reset')}：${DEFAULT_LATENCY_THRESHOLD_MS} ms`;
  if (latencyThresholdToastTimer) window.clearTimeout(latencyThresholdToastTimer);
  latencyThresholdToastTimer = window.setTimeout(() => {
    latencyThresholdAppliedToast.value = "";
    latencyThresholdToastTimer = null;
  }, 1200);
};

const mergeLegacyInternalDomainsToRules = () => {
  const legacy = String(store.appConfig.internalDomains || "")
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean);
  const existing = String(store.appConfig.networkRules || "")
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean);

  if (legacy.length === 0) return;

  const mapped = legacy.map((line) => {
    if (line.includes(":")) return line;
    if (/^\d{1,3}(\.\d{1,3}){0,3}\.?$/.test(line)) return `ip:${line}`;
    return `domain_suffix:${line}`;
  });

  const merged = Array.from(new Set([...existing, ...mapped]));
  store.appConfig.networkRules = merged.join("\n");
};

const applyDefaultNetworkRules = async () => {
  mergeLegacyInternalDomainsToRules();
  const existing = String(store.appConfig.networkRules || "").trim();
  store.appConfig.networkRules = existing
    ? `${existing}\n${DEFAULT_NETWORK_RULES}`
    : DEFAULT_NETWORK_RULES;
  store.markDirty();
};

const resetNetworkRules = async () => {
  store.appConfig.networkRules = DEFAULT_NETWORK_RULES;
  store.markDirty();
};

const toggleWhitelistLatencyMode = async () => {
  store.forceNetworkMode = store.forceNetworkMode === "latency" ? "auto" : "latency";
};

const ensureNetworkPresets = () => {
  if (!store.appConfig.networkPresets) {
    store.appConfig.networkPresets = {
      tailscale: false,
      zerotier: false,
      frp: false,
      cloudflareTunnel: false,
      ngrok: false,
    };
  }
};

const presetMeta: Record<string, { label: string; desc: string }> = {
  tailscale: { label: "Tailscale", desc: "自动识别 .ts.net 与 100.64.x.x" },
  zerotier: { label: "ZeroTier", desc: "自动识别 .zerotier.net" },
  frp: { label: "FRP", desc: "启用后可叠加你的自定义 FRP 域名规则" },
  cloudflareTunnel: { label: "Cloudflare Tunnel", desc: "识别 trycloudflare 域名" },
  ngrok: { label: "ngrok", desc: "识别 ngrok 域名" },
};

const presetKeys = Object.keys(NETWORK_PRESET_RULES);

const toggleNetworkPreset = async (key: string) => {
  ensureNetworkPresets();
  const current = !!store.appConfig.networkPresets?.[key as keyof NonNullable<typeof store.appConfig.networkPresets>];
  store.appConfig.networkPresets![key as keyof NonNullable<typeof store.appConfig.networkPresets>] = !current;
  store.markDirty();
};

const showWallpaperLibrary = ref(false);
const currentHour = ref(new Date().getHours());
let daylightTimer: number | null = null;
const updateHour = () => {
  currentHour.value = new Date().getHours();
};
const isNightTime = computed(() => currentHour.value >= 18 || currentHour.value < 6);
const isNightDaylightMode = computed(
  () => store.appConfig.daylightModeEnabled && isNightTime.value,
);
const daylightMaskPercent = computed({
  get: () => Math.round((store.appConfig.daylightMask ?? 0.5) * 100),
  set: (val: number) => {
    const v = Number.isFinite(val) ? val : 50;
    const clamped = Math.min(100, Math.max(0, v));
    store.appConfig.daylightMask = clamped / 100;
    store.markDirty();
  },
});
const musicVolume = useStorage<number>("flat-nas-music-volume", 0.7);
const musicVolumePercent = computed({
  get: () => Math.round((musicVolume.value ?? 0.7) * 100),
  set: (val: number) => {
    const v = Number.isFinite(val) ? val : 70;
    musicVolume.value = Math.min(1, Math.max(0, v / 100));
  },
});
const cardBorderHoverPreview = computed(() =>
  store.appConfig.cardBorderColor && store.appConfig.cardBorderColor !== "transparent"
    ? store.appConfig.cardBorderColor
    : store.appConfig.background || store.appConfig.solidBackgroundColor
      ? "rgba(255, 255, 255, 0.35)"
      : "rgba(15, 23, 42, 0.12)",
);
const styleVariableRows = computed(() => [
  {
    name: "--group-title-color",
    desc: "分组标题文字颜色",
    value: store.appConfig.groupTitleColor || "#ffffff",
  },
  {
    name: "--card-bg-color",
    desc: "卡片背景颜色",
    value: store.appConfig.cardBgColor || "transparent",
  },
  {
    name: "--card-border-color",
    desc: "卡片边框颜色",
    value: store.appConfig.cardBorderColor || "transparent",
  },
  {
    name: "--card-border-hover-color",
    desc: "卡片边框悬停颜色",
    value: cardBorderHoverPreview.value,
  },
  {
    name: "--card-title-color",
    desc: "卡片标题文字颜色",
    value: store.appConfig.cardTitleColor || "#111827",
  },
]);
const styleVariableStatus = {
  contrast: "待校验",
  visual: "未配置",
};
const solidBackgroundColorProxy = computed({
  get: () => store.appConfig.solidBackgroundColor || "#f3f4f6",
  set: (val: string) => {
    store.appConfig.solidBackgroundColor = val;
    store.markDirty();
  },
});

const setSolidColorAsWallpaper = () => {
  const color = store.appConfig.solidBackgroundColor || "#f3f4f6";
  if (!color) return;

  const canvas = document.createElement("canvas");
  canvas.width = 1;
  canvas.height = 1;
  const ctx = canvas.getContext("2d");
  if (ctx) {
    ctx.fillStyle = color;
    ctx.fillRect(0, 0, 1, 1);
    const dataUrl = canvas.toDataURL("image/png");
    store.appConfig.background = dataUrl;
    store.appConfig.solidBackgroundColor = "";
    store.markDirty();
  }
};

const handleWallpaperSelect = (payload: { url: string; type: string } | string) => {
  const url = typeof payload === "string" ? payload : payload.url;
  const type = typeof payload === "string" ? "pc" : payload.type;

  if (type === "mobile") {
    store.appConfig.mobileBackground = url;
  } else {
    store.appConfig.background = url;
  }
  store.markDirty();
};

const activeTab = ref("style");
const handleNavWheel = (e: WheelEvent) => {
  if (window.innerWidth < 768) {
    const container = e.currentTarget as HTMLElement;
    container.scrollLeft += e.deltaY;
  }
};

const dockerWidget = computed(() => store.widgets.find((w) => w.type === "docker"));
const systemStatusWidget = computed(() => store.widgets.find((w) => w.type === "system-status"));
const musicWidget = computed(() => store.widgets.find((w) => w.type === "music"));
const multiOpenWidgetTypes = ["iframe", "countdown", "countup", "amap-weather"];
const dedicatedWidgetTypes = ["docker"];
const isSingleOpenWidget = (type: string) =>
  !multiOpenWidgetTypes.includes(type) && !dedicatedWidgetTypes.includes(type);
const isDockerComponentCreated = computed(() => !!dockerWidget.value);
const isDockerComponentEnabled = computed(() => !!dockerWidget.value && dockerWidget.value.enable);
const sortedWidgets = computed(() => {
  const list = [...store.widgets];
  const playerIndex = list.findIndex((w) => w.type === "player");
  if (playerIndex > -1) {
    const [player] = list.splice(playerIndex, 1);
    if (player) {
      list.unshift(player);
    }
  }
  return list;
});
const singleOpenWidgets = computed(() =>
  sortedWidgets.value.filter((w) => isSingleOpenWidget(w.type)),
);
const shouldShowSingleWidgetDiagnostic = computed(() => singleOpenWidgets.value.length <= 2);
const singleWidgetDiagnosticTypes = computed(() =>
  singleOpenWidgets.value.map((w) => w.type).join("、") || "无",
);

// Debug Active Tab
watch(activeTab, (val) => {
  console.log("Active Tab Changed:", val);
});

const iconSrc = ref("/ICON.PNG");

// Ensure Docker Widget Exists
onMounted(async () => {
  if (import.meta.env.MODE === "test") return;

  // 移除强制恢复逻辑，避免覆盖用户配置
  // const hasDocker = store.widgets.some((w) => w.type === "docker");
  // if (!hasDocker) { ... }
  updateHour();
  if (daylightTimer) window.clearInterval(daylightTimer);
  daylightTimer = window.setInterval(updateHour, 60 * 1000);
});

onUnmounted(() => {
  if (daylightTimer) window.clearInterval(daylightTimer);
  daylightTimer = null;
});

const testWeatherResult = ref<{ success: boolean; message: string } | null>(null);
const isTestingWeather = ref(false);

const tempInputs = ref<Record<string, string>>({});

const updateTempInput = (key: string, value: string) => {
  tempInputs.value[key] = value;
};

const confirmTempInput = (w: WidgetConfig, field: string, key: string) => {
  if (tempInputs.value[key] !== undefined) {
    if (!w.data) w.data = {};
    w.data[field] = tempInputs.value[key];
    store.markDirty();
    // Clear temp input so it falls back to the saved value
    delete tempInputs.value[key];
  }
};

const testMusicAuthResult = ref<{ success: boolean; message: string } | null>(null);
const isTestingMusicAuth = ref(false);

const selectedMusicSize = ref({ cols: 1, rows: 1 });

watch(
  () => [musicWidget.value?.colSpan, musicWidget.value?.rowSpan],
  ([cols, rows]) => {
    if (cols && rows) {
      selectedMusicSize.value = { cols: Number(cols), rows: Number(rows) };
    }
  },
  { immediate: true },
);

const scrollToMusicSettings = () => {
  activeTab.value = "lucky-stun";
  setTimeout(() => {
    const el = document.getElementById("music-settings");
    if (el) {
      el.scrollIntoView({ behavior: "smooth", block: "start" });
    }
  }, 100);
};

const setMusicSize = (cols: number, rows: number) => {
  const index = store.widgets.findIndex((w) => w.type === "music");
  if (index !== -1) {
    const oldWidget = store.widgets[index];
    if (!oldWidget) return;

    const newLayouts = oldWidget.layouts ? JSON.parse(JSON.stringify(oldWidget.layouts)) : {};

    // Update desktop layout if it exists to ensure GridPanel picks up the change
    if (newLayouts.desktop) {
      newLayouts.desktop.w = cols;
      newLayouts.desktop.h = rows;
    }

    store.widgets[index] = {
      ...oldWidget,
      id: oldWidget.id || "",
      type: oldWidget.type || "music",
      enable: oldWidget.enable ?? true,
      colSpan: cols,
      rowSpan: rows,
      w: cols,
      h: rows,
      layouts: newLayouts,
    };
    store.widgets = [...store.widgets]; // Force reactivity for GridPanel watcher
    store.markDirty();
  }
};

const getApiBase = (url?: string) => {
  let base = url || "/api";
  base = base.replace(/\/$/, "");
  if (!/\/api(\/v\d+)?$/.test(base)) base += "/api";
  return base;
};

const testMusicAuth = async () => {
  if (!musicWidget.value) return;

  const username = musicWidget.value.data.username;
  const password = musicWidget.value.data.password;

  if (!username || !password) {
    testMusicAuthResult.value = { success: false, message: t('settings.validation.enterUsernamePassword') };
    return;
  }

  isTestingMusicAuth.value = true;
  testMusicAuthResult.value = null;

  try {
    const apiBase = getApiBase(musicWidget.value.data.apiUrl);
    const res = await fetch(`${apiBase}/auth/login`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email: username, password }),
    });

    if (res.ok) {
      const data = await res.json();
      if (data.token) {
        musicWidget.value.data.token = data.token;

        // Fetch User Profile
        try {
          const profileRes = await fetch(`${apiBase}/auth/profile`, {
            headers: { Authorization: `Bearer ${data.token}` },
          });
          if (profileRes.ok) {
            const profile = await profileRes.json();
            musicWidget.value.data.userProfile = profile;
          }
        } catch (e) {
          console.error("Failed to fetch profile", e);
        }

        store.markDirty();
        testMusicAuthResult.value = { success: true, message: t('settings.messages.musicAuth.success') };
      } else {
        testMusicAuthResult.value = { success: false, message: t('settings.messages.musicAuth.noToken') };
      }
    } else {
      const errText = await res.text();
      testMusicAuthResult.value = {
        success: false,
        message: `${t('settings.messages.loginFailed')} (${res.status}): ${errText}`,
      };
    }
  } catch (e: unknown) {
    console.error("Auth test error:", e);
    testMusicAuthResult.value = { success: false, message: `${t('settings.messages.requestError')}: ${(e as Error).message}` };
  } finally {
    isTestingMusicAuth.value = false;
  }
};

const isUpdatingProfile = ref(false);
const showRenameModal = ref(false);
const newDisplayName = ref("");

const openRenameModal = () => {
  if (!musicWidget.value || !musicWidget.value.data?.token) return;
  newDisplayName.value =
    musicWidget.value.data.userProfile?.displayName || musicWidget.value.data.username || "";
  showRenameModal.value = true;
};

const updateDisplayName = async () => {
  if (!musicWidget.value || !musicWidget.value.data?.token) return;
  const nextName = newDisplayName.value.trim();
  if (!nextName) return;

  isUpdatingProfile.value = true;
  try {
    const apiBase = getApiBase(musicWidget.value.data.apiUrl);
    const res = await fetch(`${apiBase}/auth/myprofile`, {
      method: "PATCH",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${musicWidget.value.data.token}`,
      },
      body: JSON.stringify({ displayName: nextName }),
    });

    if (res.ok) {
      const updated = await res.json();
      // Update local profile
      if (!musicWidget.value.data.userProfile) musicWidget.value.data.userProfile = updated;
      musicWidget.value.data.userProfile.displayName = updated.displayName || nextName;
      store.markDirty();
      // alert("昵称修改成功");
      showRenameModal.value = false;
    } else {
      alert(`${t('settings.messages.nicknameFailed')}: ` + (await res.text()));
    }
  } catch (e: unknown) {
    alert(`${t('settings.messages.requestError')}: ` + (e as Error).message);
  } finally {
    isUpdatingProfile.value = false;
  }
};

const getMusicAvatarUrl = (path?: string) => {
  if (!path) return "";
  if (path.startsWith("http")) return path;
  const apiBase = getApiBase(musicWidget.value?.data?.apiUrl);
  // If it's a relative path like /static/..., prepend API base
  const url = path.startsWith("/") ? `${apiBase}${path}` : `${apiBase}/${path}`;
  return store.getAssetUrl(url);
};

const testWeatherConnection = async (sourceOverride?: string) => {
  isTestingWeather.value = true;
  testWeatherResult.value = null;

  const source = sourceOverride || store.appConfig.weatherSource || "uapi";
  // Use "Shanghai" as a safe default for testing connectivity
  const city = "Shanghai";
  
  let url = "";
  if (source === "qweather") {
    const projectId = store.appConfig.qweatherProjectId || "";
    const keyId = store.appConfig.qweatherKeyId || "";
    const privateKey = store.appConfig.qweatherPrivateKey || "";
    url = `/api/weather?city=${encodeURIComponent(city)}&source=qweather&projectId=${encodeURIComponent(projectId)}&keyId=${encodeURIComponent(keyId)}&privateKey=${encodeURIComponent(privateKey)}`;
  } else if (source === "amap") {
    const key = store.appConfig.amapKey || "";
    url = `/api/weather?city=${encodeURIComponent(city)}&source=amap&key=${encodeURIComponent(key)}`;
  } else {
    // Default / UAPI (OpenMeteo)
    url = `/api/weather?city=${encodeURIComponent(city)}&source=uapi`;
  }

  try {
    const res = await fetch(url);
    const j = await res.json();
    if (res.ok && j.success && j.data) {
      testWeatherResult.value = {
        success: true,
        message: `${t('settings.messages.weatherSuccess')} ${j.data.city} ${t('settings.messages.weather')}: ${j.data.text} ${j.data.temp}°C`,
      };
    } else {
      testWeatherResult.value = {
        success: false,
        message: `${t('settings.messages.weatherFailed')}: ${j.error || t('settings.messages.weatherUnknown')}`,
      };
    }
  } catch (e) {
    testWeatherResult.value = {
      success: false,
      message: `${t('settings.messages.weatherError')}: ${(e as Error).message || String(e)}`,
    };
  } finally {
    isTestingWeather.value = false;
  }
};

const passwordInput = ref("");
const newPasswordInput = ref("");
const hasAdminAccess = computed(
  () => store.isLogged && (store.systemConfig.authMode === "single" || store.username === "admin"),
);
const canManageUsers = computed(() => hasAdminAccess.value && store.systemConfig.authMode === "multi");
const isDockerSystemEnabled = computed(() => Boolean(store.systemConfig.enableDocker));
const isUpdatingDockerSystem = ref(false);

const toggleDockerSystemEnabled = async (checked: boolean) => {
  if (isUpdatingDockerSystem.value) return;
  const current = Boolean(store.systemConfig.enableDocker);
  if (current === checked) return;

  isUpdatingDockerSystem.value = true;
  try {
    const success = await store.updateSystemConfig({ enableDocker: checked });
    if (!success) {
      alert(t('settings.messages.dockerSwitchFailed'));
    }
  } finally {
    isUpdatingDockerSystem.value = false;
  }
};

const toggleDockerMock = (checked: boolean) => {
  const w = dockerWidget.value;
  if (w) {
    if (!w.data) w.data = {};
    w.data.useMock = checked;
    store.markDirty();
  }
};

// Delete Confirmation Logic
const showDeleteWidgetConfirm = ref(false);
const widgetToDeleteId = ref("");
const editingOpacityId = ref<string | null>(null);

const confirmRemoveWidget = () => {
  const index = store.widgets.findIndex((w) => w.id === widgetToDeleteId.value);
  if (index > -1) {
    store.widgets.splice(index, 1);
    store.markDirty();
  }
  showDeleteWidgetConfirm.value = false;
  widgetToDeleteId.value = "";
};
const fileInput = ref<HTMLInputElement | null>(null);
const uploadStatus = ref("");
const musicManagerOpen = ref(false);
const isMusicListLoading = ref(false);
const musicFiles = ref<string[]>([]);
const musicManagerStatus = ref("");

const fetchMusicFiles = async () => {
  isMusicListLoading.value = true;
  musicManagerStatus.value = "";
  try {
    const res = await fetch("/api/music-list");
    if (!res.ok) throw new Error(String(res.status));
    const list = (await res.json()) as unknown;
    musicFiles.value = Array.isArray(list) ? list.map((x) => String(x)) : [];
  } catch {
    musicFiles.value = [];
    musicManagerStatus.value = t('settings.messages.fetchFailed');
  } finally {
    isMusicListLoading.value = false;
  }
};

const toggleMusicManager = async () => {
  musicManagerOpen.value = !musicManagerOpen.value;
  if (musicManagerOpen.value) {
    await fetchMusicFiles();
  }
};

const deleteMusicFile = async (filePath: string) => {
  if (!filePath) return;
  // Removed native confirm as per user request for cleaner UI
  try {
    const token = localStorage.getItem("flat-nas-token");
    const headers: Record<string, string> = { "Content-Type": "application/json" };
    if (token) headers["Authorization"] = `Bearer ${token}`;

    const res = await fetch("/api/music", {
      method: "DELETE",
      headers,
      body: JSON.stringify({ path: filePath }),
    });
    if (!res.ok) {
      const data = await res.json().catch(() => ({}));
      throw new Error(data.error || String(res.status));
    }
    await fetchMusicFiles();
  } catch (e: unknown) {
    console.error(e);
    const msg = e instanceof Error ? e.message : String(e);
    alert(`${t('settings.messages.deleteFailed')}: ${msg}`);
  }
};

const uploadMusic = async (event: Event) => {
  const files = (event.target as HTMLInputElement).files;
  if (!files || files.length === 0) return;

  const formData = new FormData();
  for (let i = 0; i < files.length; i++) {
    const file = files[i];
    if (file) {
      formData.append("files", file);
    }
  }

  uploadStatus.value = `${t('settings.messages.uploading')} ${files.length} ${t('settings.messages.files')}`;
  try {
    const token = localStorage.getItem("flat-nas-token");
    const headers: Record<string, string> = {};
    if (token) headers["Authorization"] = `Bearer ${token}`;

    const res = await fetch("/api/music/upload", {
      method: "POST",
      headers,
      body: formData,
    });
    const data = await res.json();
    if (data.success) {
      uploadStatus.value = `${t('settings.messages.uploadSuccess')} ${data.count} ${t('settings.messages.files')}`;
      if (musicManagerOpen.value) {
        await fetchMusicFiles();
      }
      setTimeout(() => {
        uploadStatus.value = "";
      }, 3000);
    } else {
      uploadStatus.value = `${t('settings.messages.uploadFailed')}: ${data.error || t('settings.messages.weatherUnknown')}`;
    }
  } catch (e) {
    console.error(e);
    uploadStatus.value = t('settings.messages.uploadError');
  }
};

const formatDockerConnectionError = (error: string, socketPath?: string) => {
  const lower = error.toLowerCase();
  if (lower.includes("docker is disabled")) {
    return "Docker 服务已关闭。\n请先打开“Docker 服务”总开关，再进行连接测试。";
  }
  if (lower.includes("docker not available")) {
    return `Docker 未启用或未配置连接地址。\n容器部署请挂载 /var/run/docker.sock 并设置 dockerHost=unix:///var/run/docker.sock\nSocket: ${socketPath || "-"}`;
  }
  if (lower.includes("docker.sock") || lower.includes("unix:///var/run/docker.sock")) {
    return `无法连接 Docker Socket，请确认宿主机 Docker 已启动，并在容器中挂载 /var/run/docker.sock\nSocket: ${socketPath || "-"}`;
  }
  return `连接失败: ${error}\nSocket: ${socketPath || "-"}`;
};

const checkDockerConnection = async () => {
  if (!isDockerSystemEnabled.value) {
    alert("Docker 服务当前已关闭。\n开启总开关后才会尝试连接 Docker Engine。");
    return;
  }
  try {
    const headers = store.getHeaders();
    const res = await fetch("/api/docker/info", { headers });
    const data = await res.json();
    if (data.success && data.state === "ready") {
      alert(
        `连接成功!\n\nSocket: ${data.socketPath}\n版本: ${data.version.Version}\n系统: ${data.info.OSType} / ${data.info.Architecture}\n容器: ${data.info.Containers}\n名称: ${data.info.Name}`,
      );
    } else if (data.state === "disabled") {
      alert("Docker 服务已关闭。\n请先打开“Docker 服务”总开关。");
    } else {
      alert(formatDockerConnectionError(data.error || "Docker 不可用", data.socketPath));
    }
  } catch (e: unknown) {
    const msg = e instanceof Error ? e.message : String(e);
    alert("网络错误: " + msg);
  }
};

const isExportingDockerLogs = ref(false);
const exportDockerLogs = async () => {
  if (isExportingDockerLogs.value) return;
  if (!isDockerSystemEnabled.value) {
    alert("Docker 服务当前已关闭，已停止日志导出请求。");
    return;
  }
  try {
    isExportingDockerLogs.value = true;
    const headers = store.getHeaders();
    const res = await fetch("/api/docker/export-logs", { headers });
    if (!res.ok) {
      const data = await res.json().catch(() => ({}));
      throw new Error(data.error || String(res.status));
    }
    const blob = await res.blob();
    const url = URL.createObjectURL(blob);
    const ts = new Date().toISOString().replace(/[:.]/g, "-");
    const link = document.createElement("a");
    link.href = url;
    link.download = `docker-logs-${ts}.json`;
    document.body.appendChild(link);
    link.click();
    link.remove();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  } catch (e: unknown) {
    const msg = e instanceof Error ? e.message : String(e);
    alert(`${t('settings.messages.exportFailed')}: ${msg}`);
  } finally {
    isExportingDockerLogs.value = false;
  }
};

// Password Confirm Logic
const showPasswordConfirm = ref(false);
const showMultiUserWarning = ref(false);
const pendingAction = ref<(() => void | Promise<void>) | null>(null);
const confirmTitle = ref("");

const requestAuth = (action: () => void | Promise<void>, title: string) => {
  pendingAction.value = action;
  confirmTitle.value = title;
  showPasswordConfirm.value = true;
};

const onAuthSuccess = async () => {
  const action = pendingAction.value;
  pendingAction.value = null;
  if (!action) return;
  await action();
};

const close = () => emit("update:show", false);

const showPassword = ref(false);

const handleLogin = async () => {
  try {
    const success = await store.login("admin", passwordInput.value);
    if (success) {
      alert(t('settings.messages.loginSuccess'));
      passwordInput.value = "";
    }
  } catch (e: unknown) {
    const msg = e instanceof Error ? e.message : t('settings.validation.enterPassword');
    alert(msg);
  }
};
const handleChangePassword = () => {
  if (!newPasswordInput.value || newPasswordInput.value.length < 4) return alert(t('settings.validation.passwordMin'));
  requestAuth(async () => {
    store.changePassword(newPasswordInput.value);
    store.markDirty();
    await store.saveData(true);
    alert(t('settings.messages.passwordChanged'));
    newPasswordInput.value = "";
  }, t('settings.messages.confirmPassword'));
};

/* ============ 云栖阁：Token / AI 模型 / OTP 二次验证 / 可信设备 ============ */
const yunqiToken = ref("");
const showYunqiToken = ref(false);
const tokenLoading = ref(false);
const maskedToken = computed(() =>
  yunqiToken.value ? "••••••••" + yunqiToken.value.slice(-4) : "——",
);
// MCP 端点与 OpenAPI 基址（按当前访问地址推导）
const mcpEndpoint = computed(() => `${window.location.origin}/mcp`);
const openapiBase = computed(() => `${window.location.origin}/openapi/v1`);
const mcpToolCount = ref(12);
// AI 智能安装提示词：用户复制后发给自己的 AI 助手，AI 照说明自动完成 MCP 接入
const mcpInstallPrompt = computed(() => `请帮我把「云栖阁」导航面板的 MCP 服务接入到你的工具列表里，接入后你就能帮我搜索、收藏、管理网址。

接入参数：
- 服务名称：yunqi-pavilion（云栖阁）
- 协议：MCP（Model Context Protocol），Streamable HTTP，JSON-RPC 2.0
- 端点地址：${mcpEndpoint.value}
- 请求方式：POST，Content-Type: application/json
- 认证方式：请求头 Authorization: Bearer ${yunqiToken.value || "<在云栖阁设置页复制的钥匙>"}
（也可以用请求头 token: <钥匙>，二选一即可）

接入步骤：
1. 在你的 MCP 配置里新增一个远程（HTTP）服务器，名称 yunqi-pavilion，地址填上面的端点，请求头带上 Authorization。
2. 调用 initialize 方法握手，再调用 tools/list 确认能看到这 12 个工具：search_bookmarks（搜书签）、list_categories（分类列表）、list_bookmarks（分类下的书签）、get_bookmark（书签详情）、create_bookmark（新增书签）、update_bookmark（修改书签）、delete_bookmark（删除书签）、checkin_sites（签到站点）、checkin_today_status（今日签到状态）、checkin_mark_done（标记已签到）、ai_ask（问云栖阁内置 AI）、system_status（系统状态）。
3. 测试一下：用 search_bookmarks 搜一个常见关键词，确认能返回结果。
4. 告诉我接入成功，以后我说「收藏这个网址到云栖阁」「云栖阁里有没有某某网站」时，你就用这些工具帮我操作。

注意：这把钥匙能操作我的全部书签，请只配置在我自己的设备上，不要分享给别人。`);
const copyMcpPrompt = async () => {
  try {
    await navigator.clipboard.writeText(mcpInstallPrompt.value);
    alert("提示词已复制，去发给你的 AI 助手吧");
  } catch {
    alert("复制失败，请手动选中复制");
  }
};

// ---- 云栖阁：音乐（LX 引擎）配置 ----
import {
  fetchMusicConfig as yqFetchMusicConfig,
  saveMusicConfig as yqSaveMusicConfig,
  fetchSources as yqFetchSources,
  importSource as yqImportSource,
  deleteSource as yqDeleteSource,
  type MusicConfig as YqMusicConfig,
  type SourceScript as YqSourceScript,
} from "@/utils/music";
const lxConfig = ref<YqMusicConfig>({
  engineUrl: "http://music-engine:23400",
  proxyEnabled: true,
  proxyHost: "192.168.57.228",
  proxyPort: 7890,
  downloadDir: "/downloads",
  defaultSource: "kw",
  defaultQuality: "320k",
  defaultMode: "play",
  lyricDownload: false,
});
const lxSources = ref<YqSourceScript[]>([]);
const lxSourceUrl = ref("");
const lxSaving = ref(false);
const lxSaveMsg = ref("");
const lxSaveOk = ref(true);
const lxImporting = ref(false);
const lxImportMsg = ref("");
const lxPresetSources = [
  { name: "ChangQing", url: "https://raw.githubusercontent.com/pdone/lx-music-source/main/changqing/latest.js" },
  { name: "Flower", url: "https://raw.githubusercontent.com/pdone/lx-music-source/main/flower/latest.js" },
  { name: "Grass", url: "https://raw.githubusercontent.com/pdone/lx-music-source/main/grass/latest.js" },
  { name: "HuanYin", url: "https://raw.githubusercontent.com/pdone/lx-music-source/main/huanyin/latest.js" },
  { name: "Huibq", url: "https://raw.githubusercontent.com/pdone/lx-music-source/main/huibq/latest.js" },
  { name: "ikun", url: "https://raw.githubusercontent.com/pdone/lx-music-source/main/ikun/latest.js" },
  { name: "JuheApi", url: "https://raw.githubusercontent.com/pdone/lx-music-source/main/juhe/latest.js" },
  { name: "LX", url: "https://raw.githubusercontent.com/pdone/lx-music-source/main/lx/latest.js" },
  { name: "QDY", url: "https://raw.githubusercontent.com/pdone/lx-music-source/main/qdy/latest.js" },
  { name: "SixYin", url: "https://raw.githubusercontent.com/pdone/lx-music-source/main/sixyin/latest.js" },
  { name: "XingHai", url: "https://raw.githubusercontent.com/pdone/lx-music-source/main/xinghai/latest.js" },
];
const loadLxMusic = async () => {
  try { lxConfig.value = await yqFetchMusicConfig(); } catch { /* 引擎未就绪时用默认值 */ }
  loadLxSources();
};
const loadLxSources = async () => {
  try { lxSources.value = await yqFetchSources(); } catch { lxSources.value = []; }
};
const saveLxConfig = async () => {
  lxSaving.value = true;
  lxSaveMsg.value = "";
  try {
    await yqSaveMusicConfig(lxConfig.value);
    lxSaveOk.value = true;
    lxSaveMsg.value = "已保存，代理设置已同步给音乐引擎";
  } catch (e) {
    lxSaveOk.value = false;
    lxSaveMsg.value = "保存失败：" + (e as Error).message;
  } finally {
    lxSaving.value = false;
  }
};
const importLxSource = async (url?: string) => {
  const u = (url || lxSourceUrl.value).trim();
  if (!u) return;
  lxImporting.value = true;
  lxImportMsg.value = "正在导入…";
  try {
    lxSources.value = await yqImportSource(u);
    const bad = lxSources.value.filter((s) => !s.ok);
    lxImportMsg.value = bad.length ? "导入完成，但有脚本加载失败，见列表" : "导入成功";
    if (!url) lxSourceUrl.value = "";
  } catch (e) {
    lxImportMsg.value = "导入失败：" + (e as Error).message;
  } finally {
    lxImporting.value = false;
  }
};
const removeLxSource = async (id: string) => {
  if (!confirm("删除这个音源脚本？")) return;
  try { lxSources.value = await yqDeleteSource(id); } catch (e) { alert("删除失败：" + (e as Error).message); }
};
// ===== VPS 监控（1Panel 接口采集） =====
interface YqVpsNode { id: string; name: string; baseUrl: string; apiKey: string; useProxy: boolean; hasKey?: boolean }
const vpsNodes = ref<YqVpsNode[]>([]);
const vpsIntervalSec = ref(20);
const vpsSaving = ref(false);
const vpsSaveMsg = ref("");
const vpsSaveOk = ref(true);
const vpsTestMsg = ref<Record<string, string>>({});
const vpsTestingId = ref("");
const loadVpsNodes = async () => {
  try {
    const res = await fetch("/api/vps/nodes", { headers: store.getHeaders() });
    if (res.ok) {
      const j = await res.json();
      vpsNodes.value = (j.nodes || []).map((n: Record<string, unknown>) => ({ ...n, apiKey: "" })) as YqVpsNode[];
      if (typeof j.intervalSec === "number" && j.intervalSec > 0) vpsIntervalSec.value = j.intervalSec;
    }
  } catch { /* 后端未就绪时静默 */ }
};
const addVpsNodeRow = () => {
  vpsNodes.value.push({ id: "vps-" + Date.now(), name: "", baseUrl: "", apiKey: "", useProxy: true });
};
const saveVpsNodes = async () => {
  vpsSaving.value = true;
  vpsSaveMsg.value = "";
  try {
    const res = await fetch("/api/vps/nodes", {
      method: "POST",
      headers: { ...store.getHeaders(), "Content-Type": "application/json" },
      body: JSON.stringify({ nodes: vpsNodes.value, intervalSec: Math.min(600, Math.max(10, Math.round(vpsIntervalSec.value) || 20)) }),
    });
    const j = await res.json().catch(() => ({}));
    if (!res.ok || !j.success) throw new Error(j.error || "保存失败");
    vpsSaveOk.value = true;
    vpsSaveMsg.value = `已保存 ${j.count} 个节点，采集已按新配置进行`;
    await loadVpsNodes();
  } catch (e) {
    vpsSaveOk.value = false;
    vpsSaveMsg.value = "保存失败：" + (e as Error).message;
  } finally {
    vpsSaving.value = false;
  }
};
const deleteVpsNode = async (n: YqVpsNode) => {
  if (!confirm(`删除节点「${n.name || n.baseUrl}」？`)) return;
  try {
    await fetch("/api/vps/nodes/delete", {
      method: "POST",
      headers: { ...store.getHeaders(), "Content-Type": "application/json" },
      body: JSON.stringify({ id: n.id }),
    });
    await loadVpsNodes();
  } catch (e) {
    alert("删除失败：" + (e as Error).message);
  }
};
const testVpsNode = async (n: YqVpsNode) => {
  vpsTestingId.value = n.id;
  vpsTestMsg.value = { ...vpsTestMsg.value, [n.id]: "正在连接…" };
  try {
    const res = await fetch("/api/vps/nodes/test", {
      method: "POST",
      headers: { ...store.getHeaders(), "Content-Type": "application/json" },
      body: JSON.stringify({ id: n.id, baseUrl: n.baseUrl, apiKey: n.apiKey, useProxy: n.useProxy }),
    });
    const j = await res.json();
    vpsTestMsg.value = {
      ...vpsTestMsg.value,
      [n.id]: j.success
        ? `连接成功：${j.os || ""}，延迟 ${j.latencyMs}ms，容器 ${j.containers} 个`
        : "连接失败：" + (j.error || "未知错误"),
    };
  } catch (e) {
    vpsTestMsg.value = { ...vpsTestMsg.value, [n.id]: "连接失败：" + (e as Error).message };
  } finally {
    vpsTestingId.value = "";
  }
};
const hasVpsWidget = computed(() => store.widgets.some((w) => w.type === "vps-monitor"));
const enableVpsMonitorWidget = () => {
  if (hasVpsWidget.value) return;
  store.widgets.push({
    id: "vps-monitor",
    type: "vps-monitor",
    enable: true,
    isPublic: false,
    colSpan: 4,
    rowSpan: 2,
    w: 4,
    h: 2,
  } as WidgetConfig);
  store.markDirty();
};
const scrollToVpsSettings = () => {
  activeTab.value = "lucky-stun";
  setTimeout(() => {
    const el = document.getElementById("vps-settings");
    if (el) el.scrollIntoView({ behavior: "smooth", block: "start" });
  }, 100);
};

watch(activeTab, (tab) => { if (tab === "lucky-stun") { loadLxMusic(); loadVpsNodes(); } });

const loadYunqiToken = async () => {
  tokenLoading.value = true;
  try {
    const res = await fetch("/api/yunqi/token", { headers: store.getHeaders() });
    if (res.ok) {
      const data = await res.json();
      yunqiToken.value = String(data?.token || "");
    }
  } catch (e) {
    console.error("加载 Token 失败", e);
  } finally {
    tokenLoading.value = false;
  }
};

const regenerateYunqiToken = async () => {
  if (!confirm(t("common.yunqi.settings.tokenRegenConfirm"))) return;
  tokenLoading.value = true;
  try {
    const res = await fetch("/api/yunqi/token", {
      method: "POST",
      headers: store.getHeaders(),
      body: JSON.stringify({ regenerate: true }),
    });
    const data = await res.json().catch(() => ({}));
    if (!res.ok || !data?.token) throw new Error(data?.error || "重新生成失败");
    yunqiToken.value = String(data.token);
    showYunqiToken.value = true;
    alert(t("common.yunqi.settings.tokenRegened"));
  } catch (e) {
    alert(e instanceof Error ? e.message : "重新生成失败，请重试");
  } finally {
    tokenLoading.value = false;
  }
};

const copyYunqiToken = async () => {
  if (!yunqiToken.value) return;
  try {
    await navigator.clipboard.writeText(yunqiToken.value);
    alert(t("common.yunqi.settings.tokenCopied"));
  } catch {
    // 剪贴板不可用时，选中展示方便手动复制
    showYunqiToken.value = true;
  }
};

// ---- AI 聊天模型设置 ----
const aiProvider = ref("DeepSeek 开放平台");
const aiPresets = ref<Array<{name: string; base_url: string; model: string; key_url: string; homepage: string}>>([]);
const aiBaseUrl = ref("https://api.deepseek.com/v1");
const aiApiKey = ref("");
const aiModel = ref("deepseek-chat");
const aiHasKey = ref(false);
const aiSaving = ref(false);
const aiProxyEnabled = ref(false);
const aiProxyUrl = ref("http://192.168.57.228:7890");

const onAiProviderChange = () => {
  const p = aiPresets.value.find((x) => x.name === aiProvider.value);
  if (p && p.name !== "自定义（OpenAI 兼容）") {
    aiBaseUrl.value = p.base_url;
    aiModel.value = p.model;
  }
};

const loadAiConfig = async () => {
  try {
    const res = await fetch("/api/ai/config", { headers: store.getHeaders() });
    if (res.ok) {
      const data = await res.json();
      if (data?.provider) aiProvider.value = String(data.provider);
      if (data?.base_url) aiBaseUrl.value = String(data.base_url);
      if (data?.model) aiModel.value = String(data.model);
      aiHasKey.value = !!data?.has_key;
      aiProxyEnabled.value = !!data?.proxy_enabled;
      if (data?.proxy_url) aiProxyUrl.value = String(data.proxy_url);
      if (Array.isArray(data?.presets)) aiPresets.value = data.presets;
    }
  } catch (e) {
    console.error("加载 AI 配置失败", e);
  }
};

const saveAiConfig = async () => {
  if (!aiBaseUrl.value.trim()) {
    alert("请填写接口地址");
    return;
  }
  if (!aiModel.value.trim()) {
    alert("请填写模型名称");
    return;
  }
  aiSaving.value = true;
  try {
    const res = await fetch("/api/ai/config", {
      method: "POST",
      headers: store.getHeaders(),
      body: JSON.stringify({
        provider: aiProvider.value,
        base_url: aiBaseUrl.value.trim(),
        api_key: aiApiKey.value, // 为空表示不改动已有密钥
        model: aiModel.value.trim(),
        proxy_enabled: aiProxyEnabled.value,
        proxy_url: aiProxyUrl.value.trim(),
      }),
    });
    const data = await res.json().catch(() => ({}));
    if (!res.ok || data?.success === false) throw new Error(data?.error || "保存失败");
    aiApiKey.value = "";
    aiHasKey.value = true;
    alert(t("common.yunqi.settings.aiSaved"));
  } catch (e) {
    alert(e instanceof Error ? e.message : "保存失败，请重试");
  } finally {
    aiSaving.value = false;
  }
};

// ---- OTP 二次验证 ----
const otpBound = ref(false);
const otpSettingUp = ref(false);
const otpSetup = ref({ secret: "", otpauth_url: "" });
const otpCode = ref("");
const otpBusy = ref(false);

const otpBoundKey = computed(() => `yunqi-otp-bound-${store.username || "default"}`);

const restoreOtpBound = () => {
  try {
    otpBound.value = localStorage.getItem(otpBoundKey.value) === "1";
  } catch {
    otpBound.value = false;
  }
};

const startOtpSetup = async () => {
  otpBusy.value = true;
  try {
    const res = await fetch("/api/auth/otp/setup", {
      method: "POST",
      headers: store.getHeaders(),
    });
    const data = await res.json().catch(() => ({}));
    if (!res.ok || !data?.otpauth_url) throw new Error(data?.error || "获取二维码失败");
    otpSetup.value = { secret: String(data.secret || ""), otpauth_url: String(data.otpauth_url) };
    otpCode.value = "";
    otpSettingUp.value = true;
  } catch (e) {
    alert(e instanceof Error ? e.message : "获取二维码失败，请重试");
  } finally {
    otpBusy.value = false;
  }
};

const confirmOtpEnable = async () => {
  const code = otpCode.value.replace(/\D/g, "");
  if (code.length !== 6) {
    alert(t("common.yunqi.login.needCode"));
    return;
  }
  otpBusy.value = true;
  try {
    const res = await fetch("/api/auth/otp/enable", {
      method: "POST",
      headers: store.getHeaders(),
      body: JSON.stringify({ code }),
    });
    const data = await res.json().catch(() => ({}));
    if (!res.ok || !data?.success) throw new Error(data?.error || t("common.yunqi.login.otpFailed"));
    otpBound.value = true;
    otpSettingUp.value = false;
    otpSetup.value = { secret: "", otpauth_url: "" };
    otpCode.value = "";
    try {
      localStorage.setItem(otpBoundKey.value, "1");
    } catch { /* ignore */ }
    alert(t("common.yunqi.settings.otpEnabledOk"));
  } catch (e) {
    alert(e instanceof Error ? e.message : t("common.yunqi.login.otpFailed"));
  } finally {
    otpBusy.value = false;
  }
};

const disableOtp = async () => {
  const code = otpCode.value.replace(/\D/g, "");
  if (code.length !== 6) {
    alert(t("common.yunqi.login.needCode"));
    return;
  }
  otpBusy.value = true;
  try {
    const res = await fetch("/api/auth/otp/disable", {
      method: "POST",
      headers: store.getHeaders(),
      body: JSON.stringify({ code }),
    });
    const data = await res.json().catch(() => ({}));
    if (!res.ok || !data?.success) throw new Error(data?.error || t("common.yunqi.login.otpFailed"));
    otpBound.value = false;
    otpCode.value = "";
    try {
      localStorage.removeItem(otpBoundKey.value);
    } catch { /* ignore */ }
    alert(t("common.yunqi.settings.otpDisabledOk"));
  } catch (e) {
    alert(e instanceof Error ? e.message : t("common.yunqi.login.otpFailed"));
  } finally {
    otpBusy.value = false;
  }
};

// ---- 可信设备 ----
interface TrustedDevice {
  id: string;
  name: string;
  created_at: string;
  last_used_at: string;
  expires_at: string;
}
const trustedDevices = ref<TrustedDevice[]>([]);
const devicesLoading = ref(false);

const formatDeviceTime = (s: string) => {
  if (!s) return t("common.yunqi.settings.never");
  const d = new Date(s);
  return Number.isNaN(d.getTime()) ? s : d.toLocaleString();
};

const loadTrustedDevices = async () => {
  devicesLoading.value = true;
  try {
    const res = await fetch("/api/auth/devices", { headers: store.getHeaders() });
    if (res.ok) {
      const data = await res.json();
      trustedDevices.value = Array.isArray(data?.devices) ? data.devices : [];
    }
  } catch (e) {
    console.error("加载可信设备失败", e);
  } finally {
    devicesLoading.value = false;
  }
};

const revokeDevice = async (device: TrustedDevice) => {
  if (!confirm(t("common.yunqi.settings.deviceRevokeConfirm"))) return;
  try {
    const res = await fetch(`/api/auth/devices/${encodeURIComponent(device.id)}`, {
      method: "DELETE",
      headers: store.getHeaders(),
    });
    const data = await res.json().catch(() => ({}));
    if (!res.ok || data?.success === false) throw new Error(data?.error || "撤销失败");
    trustedDevices.value = trustedDevices.value.filter((d) => d.id !== device.id);
  } catch (e) {
    alert(e instanceof Error ? e.message : "撤销失败，请重试");
  }
};

const loadYunqiSecurity = () => {
  restoreOtpBound();
  void loadYunqiToken();
  void loadAiConfig();
  void loadTrustedDevices();
};

watch(activeTab, (val) => {
  if (val === "account" && store.isLogged) {
    loadYunqiSecurity();
  }
});

const onMobileDockerDisplayChange = (e: Event) => {  const checked = (e.target as HTMLInputElement | null)?.checked ?? false;
  const w = dockerWidget.value;
  if (w) {
    w.hideOnMobile = !checked;
    store.markDirty();
  }
};

const handleUltrawideChange = (e: Event) => {
  const checked = (e.target as HTMLInputElement).checked;
  if (checked) {
    alert(t('settings.messages.ultrawideTip'));
  }
};

// Admin User Management
const userList = ref<string[]>([]);
const newUser = ref("");
const newPwd = ref("");
const licenseKey = ref("");

const loadUsers = async () => {
  if (!canManageUsers.value) {
    userList.value = [];
    return;
  }
  const users = await store.fetchUsers();
  if (Array.isArray(users)) {
    userList.value = users;
  }
};

const handleAddUser = async () => {
  if (!newUser.value || !newPwd.value) return alert(t('settings.validation.enterUsernamePassword'));
  try {
    await store.addUser(newUser.value, newPwd.value);
    alert(t('settings.messages.addSuccess'));
    newUser.value = "";
    newPwd.value = "";
    loadUsers();
  } catch (e: unknown) {
    alert((e as Error).message || t('settings.messages.addSuccess'));
  }
};

const handleDeleteUser = async (u: string) => {
  if (!confirm(`${t('settings.messages.confirmDeleteUser')} ${u}？`)) return;
  try {
    await store.deleteUser(u);
    alert(t('settings.messages.deleteSuccess'));
    loadUsers();
  } catch {
    alert(t('settings.messages.deleteFailed'));
  }
};

const handleUploadLicense = async () => {
  if (!licenseKey.value) return alert(t('settings.validation.enterKey'));
  try {
    await store.uploadLicense(licenseKey.value);
    alert(t('settings.messages.licenseSuccess'));
    licenseKey.value = "";
  } catch (e: unknown) {
    alert((e as Error).message || t('settings.messages.licenseFailed'));
  }
};

// Watch for tab change to load users
watch(
  activeTab,
  (val) => {
    if (val === "account" && canManageUsers.value) {
      loadUsers();
    }
  },
  { immediate: true },
);

const toggleAuthMode = async () => {
  const currentMode = store.systemConfig.authMode;
  const newMode = currentMode === "single" ? "multi" : "single";

  if (newMode === "single") {
    if (!confirm(t('settings.messages.confirmSwitchSingle'))) return;
    requestAuth(() => performAuthModeSwitch(newMode), t('settings.messages.confirmSwitchAuth'));
  } else {
    // Show custom warning for multi-user mode switch
    showMultiUserWarning.value = true;
  }
};

const performAuthModeSwitch = async (newMode: string) => {
  const success = await store.updateSystemConfig({ authMode: newMode });
  if (success) {
    close();
    store.logout();
  } else {
    alert(t('settings.messages.switchFailed'));
  }
};

onMounted(() => {
  store.checkUpdate();
});

// 单用户模式：配置版本管理
const versionLabel = ref("");
const versions = ref<{ id: string; label: string; createdAt: number; size: number }[]>([]);
const loadingVersions = ref(false);
const isImporting = ref(false);
/** 上传阶段 | 导入完成后后台补图标 */
const importOverlayMode = ref<"upload" | "icons">("upload");
const importProgress = ref(0);
const importTotal = ref(0);

const findNavItemByIdInStore = (id: string): NavItem | null => {
  for (const g of store.groups) {
    const found = g.items.find((it) => it.id === id);
    if (found) return found;
  }
  return null;
};

// 文件传输：缩略图管理
const isRegeneratingThumbs = ref(false);
const thumbRegenerationStats = ref<{ processed: number; generated: number; errors: string[] } | null>(null);

const regenerateAllThumbs = async () => {
  if (!confirm(t('settings.messages.confirmRegenerateThumbs'))) return;
  
  isRegeneratingThumbs.value = true;
  thumbRegenerationStats.value = null;
  
  try {
    const token = localStorage.getItem("flat-nas-token");
    const headers: Record<string, string> = { "Content-Type": "application/json" };
    if (token) headers["Authorization"] = `Bearer ${token}`;
    
    const response = await fetch("/api/transfer/regenerate-thumbs", {
      method: "POST",
      headers,
    });
    
    if (response.ok) {
      const stats = await response.json();
      thumbRegenerationStats.value = {
        processed: stats.processed || 0,
        generated: stats.generated || 0,
        errors: stats.errors || [],
      };
    } else {
      alert(t('settings.messages.thumbRegenFailed'));
    }
  } catch (err) {
    console.error("Failed to regenerate thumbnails:", err);
    alert(t('settings.messages.thumbRegenFailed'));
  } finally {
    isRegeneratingThumbs.value = false;
  }
};

const fetchVersions = async () => {
  try {
    loadingVersions.value = true;
    const token = localStorage.getItem("flat-nas-token");
    const headers: Record<string, string> = { "Content-Type": "application/json" };
    if (token) headers["Authorization"] = `Bearer ${token}`;
    const r = await fetch("/api/config-versions", { headers });
    if (!r.ok) return;
    const j = await r.json();
    versions.value = j.versions || [];
  } finally {
    loadingVersions.value = false;
  }
};

const saveVersion = async () => {
  try {
    const token = localStorage.getItem("flat-nas-token");
    const headers: Record<string, string> = { "Content-Type": "application/json" };
    if (token) headers["Authorization"] = `Bearer ${token}`;
    const r = await fetch("/api/config-versions", {
      method: "POST",
      headers,
      body: JSON.stringify({ label: versionLabel.value.trim() }),
    });
    if (!r.ok) {
      const d = await r.json().catch(() => ({}));
      alert(`${t('settings.messages.saveFailed')}: ${d.error || r.status}`);
      return;
    }
    versionLabel.value = "";
    await fetchVersions();
  } catch (e) {
    console.error("[SettingsModal][SaveVersion]", e);
  }
};

const restoreVersion = async (id: string) => {
  try {
    if (!confirm(t('settings.messages.confirmRestoreVersion'))) return;
    const token = localStorage.getItem("flat-nas-token");
    const headers: Record<string, string> = { "Content-Type": "application/json" };
    if (token) headers["Authorization"] = `Bearer ${token}`;
    const r = await fetch("/api/config-versions/restore", {
      method: "POST",
      headers,
      body: JSON.stringify({ id }),
    });
    if (!r.ok) {
      const d = await r.json().catch(() => ({}));
      alert(`${t('settings.messages.restoreFailed')}: ${d.error || r.status}`);
      return;
    }
    window.location.reload();
  } catch (e) {
    console.error("[SettingsModal][RestoreVersion]", e);
  }
};

const deleteVersion = async (id: string) => {
  try {
    const token = localStorage.getItem("flat-nas-token");
    const headers: Record<string, string> = { "Content-Type": "application/json" };
    if (token) headers["Authorization"] = `Bearer ${token}`;
    const r = await fetch(`/api/config-versions/${encodeURIComponent(id)}`, {
      method: "DELETE",
      headers,
    });
    if (!r.ok) {
      const d = await r.json().catch(() => ({}));
      alert(`${t('settings.messages.deleteFailed')}: ${d.error || r.status}`);
      return;
    }
    await fetchVersions();
  } catch (e) {
    console.error("[SettingsModal][DeleteVersion]", e);
  }
};

onMounted(() => {
  if (hasAdminAccess.value && store.systemConfig.authMode === "single") {
    fetchVersions();
  } else if (store.isLogged) {
    // 非单用户模式下，也允许查看自己的版本
    fetchVersions();
  }
});

const getWebhookUrl = () => {
  return toApiUrl("/api/webhook/lucky/stun");
};

const browserHelperHandshakeLink = computed(() => {
  if (typeof window === "undefined") return "";
  const token = localStorage.getItem("flat-nas-token") || "";
  if (!token) return "";

  const origin = window.location.origin;
  const url = new URL(`${origin}/`);
  const params = new URLSearchParams();
  params.set("flatnas-helper-handshake", "1");
  params.set("origin", origin);
  params.set("token", token);
  params.set("source", "open-center");
  url.hash = params.toString();
  return url.toString();
});

const copyBrowserHelperHandshakeLink = async () => {
  const link = browserHelperHandshakeLink.value;
  if (!link) {
    alert(t('settings.messages.loginFirst'));
    return;
  }
  await navigator.clipboard.writeText(link);
  alert(t('settings.messages.copiedHandshake'));
};

const openBrowserHelperHandshakeLink = () => {
  const link = browserHelperHandshakeLink.value;
  if (!link) {
    alert(t('settings.messages.loginFirst'));
    return;
  }
  window.open(link, "_blank", "noopener,noreferrer");
};

const copyWebhookUrl = () => {
  navigator.clipboard.writeText(getWebhookUrl()).then(() => {
    alert(t('settings.messages.copiedWebhook'));
  });
};

const formatTime = (ts?: number) => {
  if (!ts) return "-";
  return new Date(ts).toLocaleString();
};

onMounted(() => {
  store.fetchLuckyStunData();
});

const isUnknownWidget = (type: string) => {
  const knownTypes = [
    "clock",
    "weather",
    "clockweather",
    "calendar",
    "memo",
    "search",
    "quote",
    "bookmarks",
    "todo",
    "calculator",
    "ip",
    "player",
    "hot",
    "rss",
    "sidebar",
    "iframe",
    "countdown",
    "countup",
    "amap-weather",
    "docker",
    "system-status",
    "status-monitor",
    "file-transfer",
    "music",
    "vps-monitor",
  ];

  return !knownTypes.includes(type);
};

const restoreMissingWidgets = () => {
  const defaultWidgets = createDefaultWidgetList(store.isLogged);

  let addedCount = 0;
  defaultWidgets.forEach((def) => {
    const exists = store.widgets.some((w) => w.type === def.type);
    if (!exists) {
      store.widgets.push(def);
      addedCount++;
    }
  });

  if (addedCount > 0) {
    store.markDirty();
    alert(`${t('settings.messages.componentsRestored')} ${addedCount} ${t('settings.messages.missingComponents')}`);
} else {
  alert(t('settings.messages.noMissingComponents'));
  }
};

const addCustomCssWidget = () => {
  const newId = "custom-css-" + Date.now();
  store.widgets.push({
    id: newId,
    type: "custom-css",
    enable: true,
    data: {
      title: t('settings.messages.customComponentTitle'),
      html: t('settings.messages.customComponentHtml'),
      css: ".my-custom-component {\n  padding: 10px;\n  background: linear-gradient(to right, #e0eafc, #cfdef3);\n  border-radius: 8px;\n  text-align: center;\n  height: 100%;\n  display: flex;\n  flex-direction: column;\n  justify-content: center;\n}\n.my-custom-component h3 {\n  margin: 0 0 5px 0;\n  color: #333;\n}",
    },
    colSpan: 1,
    rowSpan: 1,
    isPublic: true,
  });
  store.markDirty();
};

const addAmapWeatherWidget = () => {
  const newId = "amap-weather-" + Date.now();
  store.widgets.push({
    id: newId,
    type: "amap-weather",
    enable: true,
    data: { city: "110101", apiKey: "" },
    colSpan: 1,
    rowSpan: 1,
    isPublic: true,
  });
  store.markDirty();
  alert(t('settings.messages.weatherAdded'));
};

const addMusicWidget = () => {
  const newId = "music-" + Date.now();
  store.widgets.push({
    id: newId,
    type: "music",
    enable: true,
    data: { title: t('settings.messages.musicTitle'), apiUrl: "", username: "", password: "" },
    colSpan: 1,
    rowSpan: 1,
    isPublic: true,
  });
  store.markDirty();
};

const enableDockerWidget = () => {
  const def: WidgetConfig = {
    id: "docker",
    type: "docker",
    enable: true,
    isPublic: true,
    colSpan: 1,
    rowSpan: 1,
    data: { useMock: false },
  };
  const exists = store.widgets.find((w) => w.type === "docker");
  if (!exists) {
    store.widgets.push(def);
    store.markDirty();
  } else {
    exists.enable = true;
    store.markDirty();
  }
};

const toggleSystemStatusMock = (checked: boolean) => {
  const w = systemStatusWidget.value;
  if (w) {
    if (!w.data) w.data = {};
    w.data.useMock = checked;
    store.markDirty();
  }
};

const enableSystemStatusWidget = () => {
  const def: WidgetConfig = {
    id: "system-status",
    type: "system-status",
    enable: true,
    isPublic: true,
    colSpan: 1,
    rowSpan: 1,
    data: { useMock: false },
  };
  const exists = store.widgets.find((w) => w.type === "system-status");
  if (!exists) {
    store.widgets.push(def);
    store.markDirty();
  } else {
    exists.enable = true;
    store.markDirty();
  }
};

const onMobileSystemStatusDisplayChange = (e: Event) => {
  const checked = (e.target as HTMLInputElement | null)?.checked ?? false;
  const w = systemStatusWidget.value;
  if (w) {
    w.hideOnMobile = !checked;
    store.markDirty();
  }
};

const handleExport = async () => {
  try {
    // 强制立即保存，确保后端数据也是最新的
    store.markDirty();

    const backupData = {
      items: store.items,
      widgets: store.widgets,
      appConfig: store.appConfig,
      groups: store.groups,
      rssFeeds: store.rssFeeds,
      rssCategories: store.rssCategories,
    };
    const jsonString = JSON.stringify(backupData, null, 2);
    const blob = new Blob([jsonString], { type: "application/json" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `flat-nas-backup-${new Date().toISOString().substring(0, 10).replace(/-/g, "")}.json`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
  } catch (e) {
    alert(t('settings.messages.exportFailed'));
    console.error("[SettingsModal][Export] failed", e);
  }
};

const triggerImport = () => {
  fileInput.value?.click();
};

const AUTO_ICON_CHECK_TIMEOUT_MS = 1500;
const autoIconCache = new Map<string, Promise<string>>();

const checkImage = (url: string, timeoutMs = AUTO_ICON_CHECK_TIMEOUT_MS): Promise<boolean> => {
  return new Promise((resolve) => {
    const img = new Image();
    let settled = false;
    const finalize = (ok: boolean) => {
      if (settled) return;
      settled = true;
      window.clearTimeout(timer);
      img.onload = null;
      img.onerror = null;
      // Stop any pending image work once we already know the result.
      img.src = "";
      resolve(ok);
    };
    const timer = window.setTimeout(() => finalize(false), timeoutMs);
    img.onload = () => finalize(true);
    img.onerror = () => finalize(false);
    img.src = url;
  });
};

const getAutoIcon = async (url: string) => {
  if (!url) return "";
  try {
    const urlObj = new URL(url);
    const hostname = urlObj.hostname.toLowerCase();
    const cacheKey = hostname || url;
    let task = autoIconCache.get(cacheKey);

    if (!task) {
      task = (async () => {
        const candidates = [
          `https://www.favicon.vip/get.php?url=${encodeURIComponent(url)}`,
          `https://icon.bqb.cool?url=${encodeURIComponent(url)}`,
          `https://icons.duckduckgo.com/ip3/${hostname}.ico`,
          `${urlObj.origin}/favicon.ico`,
        ];
        for (const src of candidates) {
          if (await checkImage(src)) return src;
        }
        return "";
      })();
      autoIconCache.set(cacheKey, task);
    }

    return await task;
  } catch {
    // ignore
  }
  return "";
};

const handleFileChange = (event: Event) => {
  const file = (event.target as HTMLInputElement).files?.[0];
  if (!file) return;
  const reader = new FileReader();
  reader.onload = async (e: ProgressEvent<FileReader>) => {
    let sunPanelItemsNeedingIcons: NavItem[] = [];
    try {
      const content = e.target?.result as string;
      let data = JSON.parse(content);

      // SunPanel format support
      if (Array.isArray(data.icons)) {
        const newGroups: NavGroup[] = data.icons.map(
          (
            g: {
              title?: string;
              children?: {
                title?: string;
                url?: string;
                lanUrl?: string;
                description?: string;
                openMethod?: number;
                icon?: { src?: string };
              }[];
            },
            gIdx: number,
          ) => ({
            id: Date.now().toString() + "_" + gIdx,
            title: g.title || "New Group",
            items: (g.children || []).map(
              (
                c: {
                  title?: string;
                  url?: string;
                  lanUrl?: string;
                  description?: string;
                  openMethod?: number;
                  icon?: { src?: string };
                },
                cIdx: number,
              ) => {
                let icon = c.icon?.src || "";
                // Only keep absolute URLs (http/https), discard relative paths (server-local)
                if (!icon.startsWith("http")) {
                  icon = "";
                }

                return {
                  id: Date.now().toString() + "_" + gIdx + "_" + cIdx,
                  title: c.title || "New Item",
                  url: c.url || "",
                  lanUrl: c.lanUrl || "",
                  icon: icon,
                  description1: c.description || "",
                  // SunPanel: 2 usually means new tab
                  openInNewTab: c.openMethod === 2,
                  isPublic: true,
                };
              },
            ),
          }),
        );

        // SunPanel：先快速导入；图标在「导入成功」后于后台抓取并单独保存（见下方 POST 之后逻辑）
        const allItems = newGroups.flatMap((g) => g.items);
        sunPanelItemsNeedingIcons = allItems.filter((it) => !it.icon && !!(it.url || it.lanUrl));

        // Preserve existing config, append new groups
        const existingGroups = store.groups;
        const finalGroups = [...existingGroups, ...newGroups];

        data = {
          groups: finalGroups,
          items: finalGroups.flatMap((g) => g.items),
          widgets: store.widgets,
          appConfig: store.appConfig,
        };
      } else if ((!data.groups || data.groups.length === 0) && data.items) {
        const items = data.items.map((item: NavItem) => ({
          ...item,
          isPublic: item.isPublic ?? true,
        }));
        data.groups = [{ id: Date.now().toString(), title: "默认分组", items: items }];
      }
      if ("password" in data) {
        delete data.password;
      }

      isImporting.value = true;
      importOverlayMode.value = "upload";
      importProgress.value = 0;
      importTotal.value = 0;

      const token = localStorage.getItem("flat-nas-token");
      const headers: Record<string, string> = { "Content-Type": "application/json" };
      if (token) headers["Authorization"] = `Bearer ${token}`;

      const r = await fetch("/api/data/import", {
        method: "POST",
        headers,
        body: JSON.stringify(data),
      });
      if (!r.ok) throw new Error("import_post_failed:" + r.status);

      await store.fetchData();

      if (sunPanelItemsNeedingIcons.length > 0) {
        importOverlayMode.value = "icons";
        importTotal.value = sunPanelItemsNeedingIcons.length;
        importProgress.value = 0;

        const batchSize = 10;
        for (let i = 0; i < sunPanelItemsNeedingIcons.length; i += batchSize) {
          const batch = sunPanelItemsNeedingIcons.slice(i, i + batchSize);
          await Promise.all(
            batch.map(async (refItem) => {
              try {
                const live = findNavItemByIdInStore(refItem.id);
                if (!live || live.icon) return;
                const targetUrl = live.url || live.lanUrl;
                if (targetUrl) {
                  const icon = await getAutoIcon(targetUrl);
                  if (icon) live.icon = icon;
                }
              } finally {
                importProgress.value++;
              }
            }),
          );
        }

        const saveResult = await store.saveData(true, true);
        if (saveResult !== "saved" && saveResult !== "no_change") {
          console.warn("[SettingsModal][Import] post-icon saveData:", saveResult);
          if (saveResult === "conflict") {
            alert(t('settings.messages.configConflict'));
            return;
          }
        }
      }

      alert(t('settings.messages.importSuccess'));
    } catch (err) {
      alert(t('settings.messages.importFailed'));
      console.error("[SettingsModal][Import] failed", err);
    } finally {
      if (fileInput.value) fileInput.value.value = "";
      isImporting.value = false;
      importOverlayMode.value = "upload";
      importProgress.value = 0;
      importTotal.value = 0;
    }
  };
  reader.readAsText(file);
};

const saveDefaultBtnText = ref("设为默认模板");

const handleReset = async () => {
  requestAuth(async () => {
    // 密码验证通过后直接执行
    try {
      const token = localStorage.getItem("flat-nas-token");
      const headers: Record<string, string> = { "Content-Type": "application/json" };
      if (token) headers["Authorization"] = `Bearer ${token}`;

      const r = await fetch("/api/reset", {
        method: "POST",
        headers,
      });
      if (!r.ok) {
        const data = await r.json().catch(() => ({}));
        throw new Error(data.error || "reset_failed");
      }
      // 移除成功弹窗，直接刷新
      window.location.reload();
    } catch (e: unknown) {
      const err = e as Error;
      alert("恢复失败: " + (err.message || "未知错误"));
      console.error("[SettingsModal][Reset] failed", e);
    }
  }, "请输入密码以确认恢复初始化");
};

const handleSaveAsDefault = async () => {
  requestAuth(async () => {
    // 密码验证通过后直接执行
    try {
      const token = localStorage.getItem("flat-nas-token");
      const headers: Record<string, string> = { "Content-Type": "application/json" };
      if (token) headers["Authorization"] = `Bearer ${token}`;

      const r = await fetch("/api/default/save", {
        method: "POST",
        headers,
      });
      if (!r.ok) {
        const data = await r.json().catch(() => ({}));
        throw new Error(data.error || "save_default_failed");
      }

      // 移除成功弹窗，使用按钮文字反馈
      saveDefaultBtnText.value = "保存成功！";
      setTimeout(() => {
        saveDefaultBtnText.value = "设为默认模板";
      }, 2000);
    } catch (e: unknown) {
      const err = e as Error;
      alert("保存失败: " + (err.message || "未知错误"));
      console.error("[SettingsModal][SaveDefault] failed", e);
    }
  }, "请输入密码以确认保存默认模板");
};

const normalizeFileTransferWidgets = () => {
  const list = store.widgets;
  const all = list.filter((w) => w.type === "file-transfer");
  if (all.length === 0) return;

  const keep = all.find((w) => w.id === "file-transfer") || all[0]!;
  let changed = false;

  for (let i = list.length - 1; i >= 0; i--) {
    const w = list[i];
    if (w && w.type === "file-transfer" && w.id !== keep.id) {
      list.splice(i, 1);
      changed = true;
    }
  }

  if (
    keep.id !== "file-transfer" &&
    !list.some((w) => w.id === "file-transfer" && w.type !== "file-transfer")
  ) {
    keep.id = "file-transfer";
    changed = true;
  }

  if (changed) store.markDirty();
};

// 修复：移除 computed 中的副作用，改用 onMounted 初始化
onMounted(() => {
  store.widgets.forEach((w: WidgetConfig) => {
    if (w.type === "iframe" && !w.data) {
      w.data = { url: "" };
    }
  });
  normalizeFileTransferWidgets();
});

const addIframeWidget = () => {
  const newId = "w-" + Date.now();
  store.widgets.push({
    id: newId,
    type: "iframe",
    enable: true,
    data: { url: "" },
    colSpan: 2,
    rowSpan: 2,
    isPublic: true,
  });
  store.markDirty();
};

const addCountdownWidget = () => {
  const newId = "w-cd-" + Date.now();
  store.widgets.push({
    id: newId,
    type: "countdown",
    enable: true,
    data: {
      targetDate: "",
      title: "重要时刻",
      style: "card",
    },
    colSpan: 1,
    rowSpan: 1,
    isPublic: true,
  });
  store.markDirty();
};

const addCountUpWidget = () => {
  const newId = "w-cup-" + Date.now();
  store.widgets.push({
    id: newId,
    type: "countup",
    enable: true,
    data: {
      startTime: new Date().toISOString().slice(0, 16),
      title: "正计时",
      style: "card",
      isRunning: false,
      totalPauseDuration: 0,
      pauseStartTime: null,
    },
    colSpan: 1,
    rowSpan: 1,
    isPublic: true,
  });
  store.markDirty();
};

const removeWidget = (id: string) => {
  widgetToDeleteId.value = id;
  showDeleteWidgetConfirm.value = true;
};

const deleteWidget = (id: string) => {
  removeWidget(id);
};

// Wallpaper Library Logic
// Wallpaper logic moved to WallpaperLibrary.vue
// Keeping minimal code if needed, or remove completely if unused.
// Since we removed the UI that uses these functions, we can remove the functions too.
// However, to be safe and clean, I will remove the unused refs and functions.

/* Removed: wallpapers, loadingWallpapers, fetchWallpapers, deleteWallpaper, uploadWallpaperInput, triggerWallpaperUpload, handleWallpaperUpload */
/* Removed: mobileWallpapers, loadingMobileWallpapers, fetchMobileWallpapers, deleteMobileWallpaper, uploadMobileWallpaperInput, triggerMobileWallpaperUpload, handleMobileWallpaperUpload */

onMounted(() => {
  // Removed wallpaper fetches
});

// Dragging Logic
const modalPosition = ref({ x: 0, y: 0 });
const isDragging = ref(false);
const dragStart = { x: 0, y: 0 };
const initialModalPosition = { x: 0, y: 0 };

const onMouseDown = (e: MouseEvent) => {
  // Prevent dragging if clicking on interactive elements
  if ((e.target as HTMLElement).closest("button, input, textarea, a, .no-drag")) return;

  isDragging.value = true;
  dragStart.x = e.clientX;
  dragStart.y = e.clientY;
  initialModalPosition.x = modalPosition.value.x;
  initialModalPosition.y = modalPosition.value.y;

  window.addEventListener("mousemove", onMouseMove);
  window.addEventListener("mouseup", onMouseUp);
};

const onMouseMove = (e: MouseEvent) => {
  if (!isDragging.value) return;
  const dx = e.clientX - dragStart.x;
  const dy = e.clientY - dragStart.y;
  modalPosition.value.x = initialModalPosition.x + dx;
  modalPosition.value.y = initialModalPosition.y + dy;
};

const onMouseUp = () => {
  isDragging.value = false;
  window.removeEventListener("mousemove", onMouseMove);
  window.removeEventListener("mouseup", onMouseUp);
};

// Nav Dragging Logic
const navRef = ref<HTMLElement | null>(null);
const isNavDragging = ref(false);
const navStartX = ref(0);
const navScrollLeft = ref(0);

const onNavMouseDown = (e: MouseEvent) => {
  if (!navRef.value) return;
  isNavDragging.value = true;
  navStartX.value = e.pageX - navRef.value.offsetLeft;
  navScrollLeft.value = navRef.value.scrollLeft;
};

const onNavMouseMove = (e: MouseEvent) => {
  if (!isNavDragging.value || !navRef.value) return;
  e.preventDefault();
  const x = e.pageX - navRef.value.offsetLeft;
  const walk = (x - navStartX.value) * 2;
  navRef.value.scrollLeft = navScrollLeft.value - walk;
};

const onNavMouseUp = () => {
  isNavDragging.value = false;
};

watch(
  () => props.show,
  (val) => {
    if (val && activeTab.value === "account" && store.isLogged) {
      if (canManageUsers.value) {
        loadUsers();
      }
      if (store.systemConfig.authMode === "single" || !canManageUsers.value) {
        fetchVersions();
      }
    }
  },
);

watch(activeTab, (val) => {
  if (val === "account" && store.isLogged && (store.systemConfig.authMode === "single" || !canManageUsers.value)) {
    fetchVersions();
  }
});
</script>

<template>
  <OverlayMotion
    :show="show"
    :z-index="50"
    overlay-class="bg-black/30 backdrop-blur-sm p-3"
    panel-class="w-full max-w-3xl"
  >
    <div
      v-if="isImporting"
      class="fixed inset-0 z-[60] bg-black/50 backdrop-blur-sm flex items-center justify-center"
    >
      <div class="bg-white rounded-xl p-6 shadow-2xl max-w-sm w-full mx-4 text-center space-y-4">
        <div
          class="animate-spin rounded-full h-12 w-12 border-4 border-blue-500 border-t-transparent mx-auto"
        ></div>
        <div>
          <h3 class="text-lg font-bold text-gray-900">
            {{ importOverlayMode === "upload" ? $t('settings.messages.importingConfig') : $t('settings.messages.fetchingIcons') }}
          </h3>
          <p class="text-sm text-gray-500 mt-1">
            <template v-if="importOverlayMode === 'upload'">
              {{ $t('settings.messages.savingToServer') }}
            </template>
            <template v-else>
              {{ $t('settings.messages.configSavedFetchingIcons') }}
            </template>
          </p>
        </div>
        <div v-if="importOverlayMode === 'icons' && importTotal > 0" class="space-y-1">
          <div class="w-full bg-gray-200 rounded-full h-2.5 overflow-hidden">
            <div
              class="bg-blue-600 h-2.5 rounded-full transition-all duration-300 ease-out"
              :style="{ width: `${(importProgress / importTotal) * 100}%` }"
            ></div>
          </div>
          <p class="text-xs text-gray-400">{{ $t('settings.messages.iconProgress') }} {{ importProgress }} / {{ importTotal }}</p>
        </div>
      </div>
    </div>

    <div
      class="backdrop-blur-xl rounded-2xl shadow-2xl w-full max-w-3xl overflow-hidden flex flex-col md:flex-row h-[600px] md:h-[480px] relative"
      :class="isNightDaylightMode ? 'night-settings bg-slate-900/60 text-slate-100 border border-white/10' : 'bg-white/90'"
      :style="{ transform: `translate(${modalPosition.x}px, ${modalPosition.y}px)` }"
      @wheel.stop
    >
      <button
        @click="close"
        class="absolute top-4 right-4 bg-red-100 hover:bg-red-200 text-red-500 hover:text-red-900 z-10 w-7 h-7 rounded-full flex items-center justify-center shadow-sm transition-colors"
      >
        <svg
          xmlns="http://www.w3.org/2000/svg"
          class="h-4 w-4"
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
          stroke-width="2.5"
        >
          <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
        </svg>
      </button>

      <div
        class="w-full md:w-1/4 bg-transparent border-b md:border-b-0 md:border-r border-gray-100 p-3 flex flex-col md:flex-col shrink-0 cursor-move glass-panel"
        @mousedown="onMouseDown"
      >
        <h3 class="text-xl font-bold text-gray-900 mb-3 md:mb-4 px-2">{{ $t('settings.title') }}</h3>
        <nav
          ref="navRef"
          class="flex flex-row md:flex-col gap-1 md:gap-0 md:space-y-0.5 overflow-x-auto md:overflow-visible pb-1 md:pb-0 no-drag cursor-grab active:cursor-grabbing overscroll-contain"
          @mousedown="onNavMouseDown"
          @mousemove="onNavMouseMove"
          @mouseup="onNavMouseUp"
          @mouseleave="onNavMouseUp"
          @wheel.stop.passive="handleNavWheel"
        >
          <button
            @click="activeTab = 'style'"
            :class="
              activeTab === 'style'
                ? 'selected-outline text-gray-900'
                : 'border border-transparent text-gray-600 hover:bg-gray-50'
            "
            class="whitespace-nowrap md:whitespace-normal w-auto md:w-full shrink-0 text-left px-3 py-1.5 rounded-lg text-sm transition-colors"
          >
            {{ $t('settings.tabs.style') }}
          </button>
          <button
            @click="activeTab = 'widgets'"
            :class="
              activeTab === 'widgets'
                ? 'selected-outline text-gray-900'
                : 'border border-transparent text-gray-600 hover:bg-gray-50'
            "
            class="whitespace-nowrap md:whitespace-normal w-auto md:w-full shrink-0 text-left px-3 py-1.5 rounded-lg text-sm transition-colors"
          >
            {{ $t('settings.tabs.widgets') }}
          </button>

          <button
            @click="activeTab = 'universal-window'"
            :class="
              activeTab === 'universal-window'
                ? 'selected-outline text-gray-900'
                : 'border border-transparent text-gray-600 hover:bg-gray-50'
            "
            class="whitespace-nowrap md:whitespace-normal w-auto md:w-full shrink-0 text-left px-3 py-1.5 rounded-lg text-sm transition-colors"
          >
            {{ $t('settings.tabs.universalWindow') }}
          </button>
          <button
            @click="activeTab = 'docker'"
            :class="
              activeTab === 'docker'
                ? 'selected-outline text-gray-900'
                : 'border border-transparent text-gray-600 hover:bg-gray-50'
            "
            class="whitespace-nowrap md:whitespace-normal w-auto md:w-full shrink-0 text-left px-3 py-1.5 rounded-lg text-sm transition-colors"
          >
            {{ $t('settings.tabs.docker') }}
          </button>
          <button
            @click="activeTab = 'account'"
            :class="
              activeTab === 'account'
                ? 'selected-outline text-gray-900'
                : 'border border-transparent text-gray-600 hover:bg-gray-50'
            "
            class="whitespace-nowrap md:whitespace-normal w-auto md:w-full shrink-0 text-left px-3 py-1.5 rounded-lg text-sm transition-colors"
          >
            {{ $t('settings.tabs.account') }}
          </button>
          <button
            @click="activeTab = 'network'"
            :class="[
              'px-3 py-1.5 text-sm transition-colors text-left flex items-center gap-1.5',
              activeTab === 'network'
                ? 'selected-outline text-gray-900'
                : 'border border-transparent text-gray-600 hover:bg-gray-50',
            ]"
            class="whitespace-nowrap md:whitespace-normal w-auto md:w-full shrink-0 rounded-lg"
          >
            {{ $t('settings.tabs.network') }}
          </button>
          <button
            @click="activeTab = 'lucky-stun'"
            :class="
              activeTab === 'lucky-stun'
                ? 'selected-outline text-gray-900'
                : 'border border-transparent text-gray-600 hover:bg-gray-50'
            "
            class="whitespace-nowrap md:whitespace-normal w-auto md:w-full shrink-0 text-left px-3 py-1.5 rounded-lg text-sm transition-colors"
          >
            {{ $t('settings.tabs.luckyStun') }}
          </button>
          <button
            @click="activeTab = 'about'"
            :class="
              activeTab === 'about'
                ? 'selected-outline text-gray-900'
                : 'border border-transparent text-red-500 hover:bg-red-50 font-medium'
            "
            class="whitespace-nowrap md:whitespace-normal w-auto md:w-full shrink-0 text-left px-3 py-1.5 rounded-lg text-sm transition-colors"
          >
            {{ $t('settings.tabs.about') }}
          </button>
          <button
            @click="activeTab = 'language'"
            :class="
              activeTab === 'language'
                ? 'selected-outline text-gray-900'
                : 'border border-transparent text-gray-600 hover:bg-gray-50'
            "
            class="whitespace-nowrap md:whitespace-normal w-auto md:w-full shrink-0 text-left px-3 py-1.5 rounded-lg text-sm transition-colors"
          >
            {{ $t('settings.tabs.language') }}
          </button>
        </nav>
        <div v-if="iconSrc" class="mt-auto px-3 pb-3 hidden md:flex justify-center">
          <img :src="iconSrc" class="w-1/4 h-auto rounded-lg shadow-sm" alt="Custom Icon" />
        </div>
      </div>

      <div class="flex-1 flex flex-col bg-transparent overflow-hidden glass-panel">
        <div class="flex-1 p-3 overflow-y-auto overscroll-contain" @wheel.stop>
          <div v-if="activeTab === 'style'" class="space-y-4">
            <div class="bg-white/60 border border-gray-100 rounded-xl p-4">
              <h4 class="text-base font-bold mb-4 text-gray-900">{{ $t('settings.sections.basicInfo') }}</h4>
              <div class="space-y-2">
                <div>
                  <label class="text-sm font-medium text-gray-700 mb-1 block">{{ $t('settings.sections.siteTitle') }}</label>
                  <input
                    v-model="store.appConfig.customTitle"
                    type="text"
                    class="w-full px-2 py-2 border border-gray-200 rounded-xl focus:border-gray-900 outline-none text-sm"
                  />
                </div>
                <div>
                  <label class="text-sm font-medium text-gray-700 mb-1 block">{{ $t('settings.sections.backgroundImage') }}</label>
                  <div class="border border-gray-200 rounded-xl p-2 bg-white/60">
                    <IconUploader
                      v-model="store.appConfig.background"
                      @update:modelValue="store.markDirty()"
                      :crop="false"
                      :previewStyle="{
                        filter: `blur(${store.appConfig.backgroundBlur ?? 0}px)`,
                        transform: 'scale(1.1)',
                      }"
                      :overlayStyle="{
                        backgroundColor: `rgba(0,0,0,${store.appConfig.backgroundMask ?? 0})`,
                      }"
                    />
                    <div class="mt-2 flex justify-between items-center">
                      <button
                        v-if="store.appConfig.background"
                        @click="
                          store.appConfig.background = '';
                          store.markDirty();
                        "
                        class="text-xs text-red-500 hover:text-red-600 px-2 py-1 rounded bg-red-50 hover:bg-red-100 transition-colors"
                      >
                        清除背景
                      </button>
                      <button
                        @click="showWallpaperLibrary = true"
                        class="text-xs text-blue-600 hover:text-blue-700 font-bold flex items-center gap-1 ml-auto px-2 py-1 rounded bg-blue-50 hover:bg-blue-100 transition-colors"
                      >
                        {{ $t('settings.sections.manageWallpaperLibrary') }}
                      </button>
                    </div>
                  </div>
                </div>
                <div class="space-y-2">
                  <div class="flex items-center justify-between rounded-xl border border-gray-100 bg-white/70 px-3 py-2">
                    <div class="flex flex-col">
                      <span class="text-sm font-medium text-gray-700">{{ $t('settings.sections.dayMode') }}</span>
                      <span class="text-[11px] text-gray-400"
                        >{{ $t('settings.sections.dayModeDesc') }}
                        {{ daylightMaskPercent }}%</span
                      >
                    </div>
                    <div class="flex items-center gap-2">
                      <div
                        class="flex items-center gap-1 px-2 py-1 rounded-lg border border-gray-200 bg-white/80 text-gray-700"
                      >
                        <input
                          v-model.number="daylightMaskPercent"
                          type="number"
                          min="0"
                          max="100"
                          step="1"
                          class="w-12 text-xs bg-transparent outline-none"
                        />
                        <span class="text-xs">%</span>
                      </div>
                      <label class="relative inline-flex items-center cursor-pointer">
                        <input
                          type="checkbox"
                          class="sr-only peer"
                          :checked="store.appConfig.daylightModeEnabled"
                          @change="
                            (e) => {
                              store.appConfig.daylightModeEnabled = (
                                e.target as HTMLInputElement
                              ).checked;
                              store.markDirty();
                            }
                          "
                        />
                        <div
                          class="w-9 h-5 bg-gray-200 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-yellow-400"
                        ></div>
                      </label>
                    </div>
                  </div>
                  <div class="flex items-center justify-between rounded-xl border border-gray-100 bg-white/70 px-3 py-2">
                    <div class="flex flex-col">
                      <span class="text-sm font-medium text-gray-700">{{ $t('settings.sections.weatherEffect') }}</span>
                      <span class="text-[11px] text-gray-400">{{ $t('settings.sections.weatherEffectDesc') }}</span>
                    </div>
                    <label class="relative inline-flex items-center cursor-pointer">
                      <input
                        type="checkbox"
                        class="sr-only peer"
                        :checked="store.appConfig.weatherEffectEnabled"
                        @change="
                          (e) => {
                            store.appConfig.weatherEffectEnabled = (
                              e.target as HTMLInputElement
                            ).checked;
                            store.markDirty();
                          }
                        "
                      />
                      <div
                        class="w-9 h-5 bg-gray-200 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-blue-500"
                      ></div>
                    </label>
                  </div>
                </div>
                <div>
                  <label class="text-sm font-medium text-gray-700 mb-1 block">{{ $t('settings.sections.solidColor') }}</label>
                  <div class="flex items-center gap-2">
                    <input
                      type="color"
                      v-model="solidBackgroundColorProxy"
                      class="w-10 h-10 rounded cursor-pointer border-0 p-0"
                    />
                    <input
                      v-model="store.appConfig.solidBackgroundColor"
                      @change="store.markDirty()"
                      type="text"
                      placeholder="#f3f4f6"
                      class="flex-1 px-2 py-2 border border-gray-200 rounded-xl focus:border-gray-900 outline-none text-sm"
                    />
                    <button
                      @click="
                        store.appConfig.solidBackgroundColor = '';
                        store.markDirty();
                      "
                      class="px-3 h-8 rounded-lg bg-red-50 hover:bg-red-100 flex items-center justify-center text-red-500 transition-colors text-xs font-medium"
                      :title="$t('settings.extraSections.clearSolidBg')"
                    >
                      {{ $t('settings.extraSections.resetBtn') }}
                    </button>
                    <button
                      @click="setSolidColorAsWallpaper"
                      class="px-3 h-8 rounded-lg bg-blue-50 hover:bg-blue-100 flex items-center justify-center text-blue-600 transition-colors text-xs font-medium"
                      :title="$t('settings.sections.setAsWallpaper')"
                    >
                      {{ $t('settings.sections.setAsWallpaper') }}
                    </button>
                  </div>
                </div>
              </div>
            </div>

            <WallpaperLibrary v-model:show="showWallpaperLibrary" @select="handleWallpaperSelect" />

            <div class="bg-white/60 border border-gray-100 rounded-xl p-4">
              <h4 class="text-base font-bold mb-4 text-gray-900">{{ $t('settings.sections.layout') }}</h4>
              <div class="mb-2">
                <h5 class="text-sm font-medium text-gray-700 mb-2">{{ $t('settings.sections.topBarLayout') }}</h5>
                <div class="flex gap-2">
                  <button
                    @click="store.appConfig.titleAlign = 'left'"
                    class="flex-1 p-2 border-2 rounded-xl flex items-center justify-center gap-2 transition-colors"
                    :class="
                      store.appConfig.titleAlign === 'left'
                        ? 'selected-outline text-gray-900'
                        : 'border-gray-200 text-gray-500 bg-white hover:bg-gray-50'
                    "
                  >
                    <span class="text-sm">{{ $t('settings.sections.standardLayout') }}</span>
                  </button>
                  <button
                    @click="store.appConfig.titleAlign = 'right'"
                    class="flex-1 p-2 border-2 rounded-xl flex items-center justify-center gap-2 transition-colors"
                    :class="
                      store.appConfig.titleAlign === 'right'
                        ? 'selected-outline text-gray-900'
                        : 'border-gray-200 text-gray-500 bg-white hover:bg-gray-50'
                    "
                  >
                    <span class="text-sm">{{ $t('settings.sections.invertedLayout') }}</span>
                  </button>
                  <button
                    @click="
                      store.appConfig.hideHeaderOnMobile = !store.appConfig.hideHeaderOnMobile
                    "
                    class="flex flex-1 p-2 border-2 rounded-xl items-center justify-center gap-2 transition-colors"
                    :class="
                      store.appConfig.hideHeaderOnMobile
                        ? 'selected-outline text-gray-900'
                        : 'border-gray-200 text-gray-500 bg-white hover:bg-gray-50'
                    "
                  >
                    <span class="text-sm">{{ $t('settings.sections.hideHeaderOnMobile') }}</span>
                  </button>
                </div>
              </div>
              <div class="grid grid-cols-2 gap-4 mb-2">
                <div>
                  <h4 class="text-sm font-medium text-gray-700 mb-1">{{ $t('settings.sections.titleSizeLabel') }}</h4>
                  <input
                    type="range"
                    v-model.number="store.appConfig.titleSize"
                    min="20"
                    max="80"
                    class="w-full h-2 bg-gray-200 rounded-lg appearance-none cursor-pointer accent-blue-400"
                  />
                </div>
                <div>
                  <h4 class="text-sm font-medium text-gray-700 mb-1">{{ $t('settings.sections.titleColorLabel') }}</h4>
                  <div class="flex items-center gap-2">
                    <input
                      type="color"
                      v-model="store.appConfig.titleColor"
                      class="w-10 h-10 rounded cursor-pointer border-0 p-0"
                    />
                    <button
                      @click="store.appConfig.titleColor = '#ffffff'"
                      class="px-3 h-8 rounded-lg bg-red-50 hover:bg-red-100 flex items-center justify-center text-red-500 transition-colors text-xs font-medium"
                      :title="$t('settings.extraSections.resetColor')"
                    >
                      {{ $t('settings.extraSections.resetBtn') }}
                    </button>
                  </div>
                </div>
              </div>
              <div class="mb-2">
                <h5 class="text-sm font-medium text-gray-700 mb-2">{{ $t('settings.sections.webLayout') }}</h5>
                <div
                  class="flex items-center justify-between border border-gray-200 rounded-xl p-3"
                >
                  <span class="text-sm font-bold text-gray-900">{{ $t('settings.sections.displayMode') }}</span>
                  <div class="flex items-center gap-2">
                    <div
                      class="relative h-9 w-[180px] rounded-full bg-gray-100/90 p-1 flex items-center select-none shadow-inner focus-within:ring-2 focus-within:ring-green-300/60 focus-within:ring-offset-2 focus-within:ring-offset-white/60"
                    >
                      <div
                        class="absolute top-1 bottom-1 left-1 w-[calc(50%-4px)] rounded-full bg-green-500 shadow-sm shadow-green-500/25 transition-transform duration-200"
                        :class="
                          store.appConfig.webGroupPagination
                            ? 'translate-x-[calc(100%+4px)]'
                            : 'translate-x-0'
                        "
                      ></div>
                      <button
                        type="button"
                        class="relative z-10 w-1/2 h-full text-xs font-bold rounded-full transition-colors focus-visible:outline-none"
                        :class="
                          store.appConfig.webGroupPagination
                            ? 'text-gray-600 hover:text-gray-800'
                            : 'text-white'
                        "
                        @click="store.appConfig.webGroupPagination = false"
                      >
                        {{ $t('settings.sections.singleColumn') }}
                      </button>
                      <button
                        type="button"
                        class="relative z-10 w-1/2 h-full text-xs font-bold rounded-full transition-colors focus-visible:outline-none"
                        :class="
                          store.appConfig.webGroupPagination
                            ? 'text-white'
                            : 'text-gray-600 hover:text-gray-800'
                        "
                        @click="store.appConfig.webGroupPagination = true"
                      >
                        {{ $t('settings.sections.pageByGroup') }}
                      </button>
                    </div>
                    <button
                      type="button"
                      class="h-9 px-3 rounded-xl text-xs font-bold border transition-colors focus-visible:outline-none"
                      :disabled="!store.appConfig.webGroupPagination"
                      :class="
                        !store.appConfig.webGroupPagination
                          ? 'bg-gray-50 text-gray-300 border-gray-100 cursor-not-allowed'
                          : store.appConfig.webGroupPaginationDisableFlip
                            ? 'bg-red-500 text-white border-red-500 hover:bg-red-600'
                            : 'bg-white text-gray-700 border-gray-200 hover:bg-gray-50'
                      "
                      @click="
                        store.appConfig.webGroupPaginationDisableFlip =
                          !store.appConfig.webGroupPaginationDisableFlip
                      "
                    >
                      {{ $t('settings.sections.disablePagination') }}
                    </button>
                  </div>
                </div>
              </div>

              <div class="grid grid-cols-2 gap-4 mt-4">
                <div>
                  <h4 class="text-sm font-medium text-gray-700 mb-1">{{ $t('settings.sections.widgetAreaSize') }}</h4>
                  <div class="flex items-center gap-2">
                    <input
                      type="number"
                      v-model.number="store.appConfig.widgetAreaCols"
                      min="0.5"
                      step="0.5"
                      max="16"
                      class="w-20 px-2 py-2 border border-gray-200 rounded-xl focus:border-gray-900 outline-none text-sm bg-white"
                    />
                    <span class="text-xs text-gray-400 select-none">×</span>
                    <input
                      type="number"
                      v-model.number="store.appConfig.widgetAreaRows"
                      min="0.5"
                      step="0.5"
                      max="16"
                      class="w-20 px-2 py-2 border border-gray-200 rounded-xl focus:border-gray-900 outline-none text-sm bg-white"
                    />
                    <span class="ml-auto text-xs text-gray-500 w-14 text-right"
                      >{{ store.appConfig.widgetAreaCols ?? 4 }}×{{
                        store.appConfig.widgetAreaRows ?? 4
                      }}</span
                    >
                  </div>
                </div>
                <div>
                  <h4 class="text-sm font-medium text-gray-700 mb-1">{{ $t('settings.sections.groupVerticalGap') }}</h4>
                  <div class="flex items-center gap-2">
                    <input
                      type="range"
                      v-model.number="store.appConfig.groupGap"
                      min="0"
                      max="100"
                      step="5"
                      class="flex-1 h-2 bg-gray-200 rounded-lg appearance-none cursor-pointer accent-blue-400"
                    />
                    <span class="text-xs text-gray-500 w-6">{{
                      store.appConfig.groupGap ?? 30
                    }}</span>
                  </div>
                </div>
                <div>
                  <h4 class="text-sm font-medium text-gray-700 mb-1">{{ $t('settings.sections.mouseHoverEffect') }}</h4>
                  <select
                    v-model="store.appConfig.mouseHoverEffect"
                    class="w-full px-3 py-2 border border-gray-200 rounded-xl focus:border-gray-900 outline-none text-sm bg-white"
                  >
                    <option value="scale">{{ $t('settings.sections.hoverScale') }}</option>
                    <option value="lift">{{ $t('settings.sections.hoverLift') }}</option>
                    <option value="glow">{{ $t('settings.sections.hoverGlow') }}</option>
                    <option value="none">{{ $t('settings.sections.hoverNone') }}</option>
                  </select>
                </div>
              </div>
            </div>

            <div class="bg-white/60 border border-gray-100 rounded-xl p-4">
              <h4 class="text-base font-bold mb-4 text-gray-900">{{ $t('settings.sections.footerSettings') }}</h4>
              <div class="space-y-2">
                <div class="flex items-center justify-between">
                  <label class="text-sm font-medium text-gray-700">{{ $t('settings.sections.showVisitorStats') }}</label>
                  <label class="relative inline-flex items-center cursor-pointer">
                    <input
                      type="checkbox"
                      v-model="store.appConfig.showFooterStats"
                      class="sr-only peer"
                    />
                    <div
                      class="w-11 h-6 bg-gray-200 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-blue-600"
                    ></div>
                  </label>
                </div>
                <div class="grid grid-cols-2 gap-4">
                  <div>
                    <label class="text-sm font-medium text-gray-700 mb-1 block"
                      >{{ $t('settings.sections.footerHeight') }}</label
                    >
                    <div class="text-xs text-gray-500 mb-1">{{ $t('settings.sections.footerHeightDesc') }}</div>
                    <input
                      type="number"
                      v-model="store.appConfig.footerHeight"
                      class="w-full px-3 py-2 border border-gray-200 rounded-xl focus:border-gray-900 outline-none text-sm"
                      placeholder="0"
                    />
                  </div>
                  <div>
                    <label class="text-sm font-medium text-gray-700 mb-1 block"
                      >{{ $t('settings.sections.footerContentWidth') }}</label
                    >
                    <div class="text-xs text-gray-500 mb-1">{{ $t('settings.sections.footerContentWidthDesc') }}</div>
                    <input
                      type="number"
                      v-model="store.appConfig.footerWidth"
                      class="w-full px-3 py-2 border border-gray-200 rounded-xl focus:border-gray-900 outline-none text-sm"
                      placeholder="1280"
                    />
                  </div>
                </div>
                <div class="grid grid-cols-2 gap-4">
                  <div>
                    <label class="text-sm font-medium text-gray-700 mb-1 block"
                      >{{ $t('settings.sections.footerMarginBottom') }}</label
                    >
                    <div class="text-xs text-gray-500 mb-1">{{ $t('settings.sections.footerMarginBottomDesc') }}</div>
                    <input
                      type="number"
                      v-model="store.appConfig.footerMarginBottom"
                      class="w-full px-3 py-2 border border-gray-200 rounded-xl focus:border-gray-900 outline-none text-sm"
                      placeholder="0"
                    />
                  </div>
                  <div>
                    <label class="text-sm font-medium text-gray-700 mb-1 block"
                      >{{ $t('settings.sections.footerFontSize') }}</label
                    >
                    <div class="text-xs text-gray-500 mb-1">{{ $t('settings.sections.footerFontSizeDesc') }}</div>
                    <input
                      type="number"
                      v-model="store.appConfig.footerFontSize"
                      class="w-full px-3 py-2 border border-gray-200 rounded-xl focus:border-gray-900 outline-none text-sm"
                      placeholder="12"
                    />
                  </div>
                </div>
                <div>
                  <label class="text-sm font-medium text-gray-700 mb-1 block"
                    >{{ $t('settings.sections.customFooterHtml') }}</label
                  >
                  <textarea
                    v-model="store.appConfig.footerHtml"
                    rows="3"
                    :placeholder="$t('settings.sections.customFooterHtmlPlaceholder')"
                    class="w-full px-3 py-2 border border-gray-200 rounded-xl focus:border-gray-900 outline-none text-sm font-mono"
                  ></textarea>
                </div>
              </div>
            </div>
          </div>

          <div v-if="activeTab === 'widgets'" class="space-y-4">
            <div class="flex items-center justify-between mb-4 mr-8">
              <h4 class="text-base font-bold text-gray-900 border-l-4 border-gray-900 pl-3">
                {{ $t('settings.sections.desktopWidgets') }}
              </h4>
              <div class="flex items-center gap-3 text-xs mr-[10px]">
                <button
                  @click="restoreMissingWidgets"
                  class="text-gray-600 hover:text-gray-900 underline mr-2"
                >
                  恢复默认组件
                </button>
                <button
                  @click="addCustomCssWidget"
                  class="text-gray-600 hover:text-gray-900 underline mr-2"
                >
                  + 自定义组件
                </button>
              </div>
            </div>

            <div
              v-if="shouldShowSingleWidgetDiagnostic"
              class="rounded-xl border border-amber-200 bg-amber-50/80 p-4 text-sm text-amber-900"
            >
              <div class="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
                <div class="space-y-1">
                  <div class="font-bold">{{ $t('settings.sections.singleWidgetDiagnostic') }}</div>
                  <p class="text-xs leading-relaxed text-amber-800">
                    {{ $t('settings.sections.singleWidgetDiagnosticDesc', { count: singleOpenWidgets.length, total: store.widgets.length, types: singleWidgetDiagnosticTypes }) }}
                  </p>
                </div>
                <button
                  type="button"
                  @click="restoreMissingWidgets"
                  class="shrink-0 rounded-lg bg-amber-600 px-3 py-2 text-xs font-bold text-white transition-colors hover:bg-amber-700"
                >
                  恢复默认组件
                </button>
              </div>
            </div>

            <!-- Normal Widgets Grid -->
            <div class="grid grid-cols-2 md:grid-cols-4 gap-3 md:gap-4">
              <template v-for="w in singleOpenWidgets" :key="w.id">
                <div
                  :class="[
                    'border border-gray-100 rounded-xl bg-white/60 hover:shadow-md transition-all relative',
                    w.type === 'player'
                      ? 'col-span-2 md:col-span-4 flex flex-col gap-3 p-4 md:grid md:grid-cols-[auto_1fr_auto] md:items-center md:gap-4'
                      : 'flex flex-col items-stretch gap-2 p-3',
                  ]"
                >
                  <button
                    v-if="isUnknownWidget(w.type)"
                    @click="deleteWidget(w.id)"
                    class="absolute top-1 right-1 w-5 h-5 flex items-center justify-center bg-gray-100 hover:bg-gray-200 text-gray-600 rounded-full text-xs transition-colors z-20"
                    :title="$t('settings.sections.deleteWidget')"
                  >
                    ✕
                  </button>
                  <template v-if="w.type === 'player'">
                    <div class="flex items-center gap-3 flex-shrink-0">
                      <div
                        class="w-10 h-10 rounded-full bg-gray-50 flex items-center justify-center text-sm font-medium text-gray-700 shadow-sm"
                      >
                        {{ $t('settings.widgetAbbr.player') }}
                      </div>
                      <span class="font-bold text-gray-700 text-sm">{{ $t('settings.sections.randomMusic') }}</span>
                    </div>
                    <div class="flex flex-wrap items-center gap-2">
                      <label
                        class="px-3 py-1.5 text-gray-700 text-xs rounded-lg cursor-pointer transition-colors flex items-center gap-1 whitespace-nowrap glass-chip selectable-outline"
                      >
                        <span>{{ $t('settings.sections.uploadMusic') }}</span>
                        <input
                          type="file"
                          accept="audio/*"
                          class="hidden"
                          multiple
                          @change="uploadMusic"
                        />
                      </label>
                      <button
                        type="button"
                        @click="toggleMusicManager"
                        class="px-3 py-1.5 text-gray-700 text-xs rounded-lg cursor-pointer transition-colors flex items-center gap-1 whitespace-nowrap glass-chip selectable-outline"
                      >
                        {{ musicManagerOpen ? $t('settings.sections.collapseFiles') : $t('settings.sections.fileManagement') }} ({{ musicFiles.length }})
                      </button>
                      <span
                        v-if="uploadStatus"
                        class="text-xs"
                        :class="(uploadStatus.includes(t('settings.messages.uploadFailed')) || uploadStatus.includes('失败')) ? 'text-red-500' : 'text-green-500'"
                        >{{ uploadStatus }}</span
                      >
                    </div>
                    <div class="flex flex-col items-stretch gap-2 md:items-end">
                      <div class="flex items-center gap-2 justify-end">
                        <span class="text-xs text-gray-500 whitespace-nowrap">{{ $t('settings.sections.musicVolume') }}</span>
                        <input
                          v-model.number="musicVolumePercent"
                          type="range"
                          min="0"
                          max="100"
                          step="1"
                          class="w-28 h-1 bg-gray-200 rounded-lg appearance-none cursor-pointer accent-blue-400"
                        />
                      </div>
                      <div class="grid grid-cols-2 sm:grid-cols-4 gap-3">
                        <div class="flex flex-col items-center gap-0.5">
                          <span class="text-[10px] text-gray-400 scale-90">{{ $t('settings.sections.publicAccess') }}</span>
                          <label
                            class="relative inline-flex items-center cursor-pointer"
                            :title="$t('settings.sections.publicAccess')"
                            ><input type="checkbox" v-model="w.isPublic" class="sr-only peer" @change="store.saveData()" />
                            <div
                              class="w-7 h-4 bg-gray-200 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-3 after:w-3 after:transition-all peer-checked:bg-blue-500"
                            ></div
                          ></label>
                        </div>
                        <div class="flex flex-col items-center gap-0.5">
                          <span class="text-[10px] text-gray-400 scale-90">{{ $t('settings.sections.enabled') }}</span>
                          <label
                            class="relative inline-flex items-center cursor-pointer"
                            :title="$t('settings.sections.enabled')"
                            ><input type="checkbox" v-model="w.enable" class="sr-only peer" @change="store.saveData()" />
                            <div
                              class="w-7 h-4 bg-gray-200 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-3 after:w-3 after:transition-all peer-checked:bg-green-500"
                            ></div
                          ></label>
                        </div>
                        <div class="flex flex-col items-center gap-0.5">
                          <span class="text-[10px] text-gray-400 scale-90">{{ $t('settings.sections.mobileAccess') }}</span>
                          <label
                            class="relative inline-flex items-center cursor-pointer"
                            :title="$t('settings.sections.mobileAccess')"
                            ><input
                              type="checkbox"
                              :checked="!w.hideOnMobile"
                              class="sr-only peer"
                              @change="
                                (e) => {
                                  w.hideOnMobile = !(e.target as HTMLInputElement).checked;
                                  store.markDirty();
                                }
                              " />
                            <div
                              class="w-7 h-4 bg-gray-200 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-3 after:w-3 after:transition-all peer-checked:bg-orange-500"
                            ></div
                          ></label>
                        </div>
                        <div class="flex flex-col items-center gap-0.5">
                          <span class="text-[10px] text-gray-400 scale-90">{{ $t('settings.sections.autoPlay') }}</span>
                          <label
                            class="relative inline-flex items-center cursor-pointer"
                            :title="$t('settings.sections.autoPlayTitle')"
                            ><input
                              type="checkbox"
                              v-model="store.appConfig.autoPlayMusic"
                              @change="store.markDirty()"
                              class="sr-only peer" />
                            <div
                              class="w-7 h-4 bg-gray-200 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-3 after:w-3 after:transition-all peer-checked:bg-blue-600"
                            ></div
                          ></label>
                        </div>
                      </div>
                    </div>
                    <div v-if="musicManagerOpen" class="md:col-start-1 md:col-span-3 space-y-2">
                      <div class="flex items-center gap-2">
                        <button
                          type="button"
                          @click="fetchMusicFiles"
                          class="px-3 py-1.5 bg-gray-100 text-gray-700 text-xs rounded-lg cursor-pointer hover:bg-gray-200 transition-colors flex items-center gap-1 whitespace-nowrap"
                          :disabled="isMusicListLoading"
                        >
                          {{ isMusicListLoading ? $t('settings.sections.refreshing') : $t('settings.sections.refreshList') }}
                        </button>
                        <span v-if="musicManagerStatus" class="text-xs text-red-500">{{
                          musicManagerStatus
                        }}</span>
                        <span v-else class="text-xs text-gray-500"
                          >{{ $t('settings.sections.totalFiles', { count: musicFiles.length }) }}</span
                        >
                      </div>
                      <div
                        class="border border-gray-100 rounded-xl bg-gray-50 p-3 max-h-44 overflow-auto"
                      >
                        <div v-if="isMusicListLoading" class="text-xs text-gray-500">{{ $t('settings.extraSections.loadingVersions') }}</div>
                        <div v-else-if="musicFiles.length === 0" class="text-xs text-gray-500">
                          {{ $t('settings.sections.noMusicFiles') }}
                        </div>
                        <div v-else class="space-y-1">
                          <div
                            v-for="f in musicFiles"
                            :key="f"
                            class="flex items-center gap-2 text-xs"
                          >
                            <span class="flex-1 truncate text-gray-700" :title="f">{{ f }}</span>
                            <button
                              type="button"
                              class="px-2 py-1 rounded-md bg-red-50 text-red-600 hover:bg-red-100 transition-colors"
                              @click="deleteMusicFile(f)"
                            >
                              {{ $t('settings.sections.delete') }}
                            </button>
                          </div>
                        </div>
                      </div>
                    </div>
                  </template>
                  <template v-else>
                    <div
                      class="flex flex-col items-center gap-1.5 scale-100 cursor-pointer hover:bg-gray-50 rounded-lg transition-colors w-full py-1"
                      @click="
                        w.type === 'music'
                          ? scrollToMusicSettings()
                          : w.type === 'vps-monitor'
                            ? scrollToVpsSettings()
                            : w.type === 'system-status'
                              ? (activeTab = 'docker')
                              : (editingOpacityId = w.id)
                      "
                      :title="
                        w.type === 'music' || w.type === 'system-status' || w.type === 'vps-monitor'
                          ? $t('settings.sections.clickToSettings')
                          : $t('settings.sections.clickToStyle')
                      "
                    >
                      <template v-if="editingOpacityId === w.id">
                        <div class="w-full px-2" @click.stop>
                          <label class="text-[10px] text-gray-500 block mb-1"
                            >{{ $t('settings.sections.opacity') }}
                            {{
                              Math.round((w.opacity ?? (w.type === "search" ? 0.9 : 1)) * 100)
                            }}%</label
                          >
                          <input
                            type="range"
                            min="0.1"
                            max="1"
                            step="0.1"
                            :value="w.opacity ?? (w.type === 'search' ? 0.9 : 1)"
                            @input="
                              (e) => {
                                w.opacity = parseFloat((e.target as HTMLInputElement).value);
                                store.markDirty();
                              }
                            "
                            class="w-full h-1 bg-gray-200 rounded-lg appearance-none cursor-pointer accent-blue-400"
                          />
                          <div class="flex items-center justify-between mt-2 gap-2">
                            <label class="text-[10px] text-gray-500">{{ $t('settings.sections.textColor') }}</label>
                            <div class="flex items-center gap-2">
                              <input
                                type="color"
                                :value="
                                  w.textColor || (w.type === 'search' ? '#111827' : '#374151')
                                "
                                @input="
                                  (e) => {
                                    w.textColor = (e.target as HTMLInputElement).value;
                                    store.markDirty();
                                  }
                                "
                                @change="store.markDirty()"
                                class="w-5 h-5 p-0 border-0 rounded-full cursor-pointer overflow-hidden shadow-sm"
                                :title="$t('settings.extraSections.selectColor')"
                              />
                              <button
                                v-if="w.textColor"
                                @click.stop="
                                  w.textColor = undefined;
                                  store.markDirty();
                                "
                                class="text-[10px] text-red-400 hover:text-red-600"
                                :title="$t('settings.extraSections.resetColor')"
                              >
                                ✕
                              </button>
                            </div>
                          </div>
                          <div class="flex items-center gap-2 mt-2 border-t border-gray-100 pt-1">
                            <button
                              @click.stop="removeWidget(w.id)"
                              class="flex-1 text-xs text-red-500 hover:text-red-700 text-center py-0.5"
                            >
                              删除组件
                            </button>
                            <button
                              @click.stop="editingOpacityId = null"
                              class="flex-1 text-xs text-gray-600 hover:text-gray-900 text-center py-0.5"
                            >
                              {{ $t('settings.sections.done') }}
                            </button>
                          </div>
                        </div>
                      </template>
                      <template v-else>
                        <div
                          class="w-9 h-9 rounded-full bg-gray-50 flex items-center justify-center text-xs font-medium text-gray-700 shadow-sm"
                        >
                          <img
                            v-if="w.type === 'music'"
                            :src="daoliyuLogo"
                            class="w-6 h-6 object-contain"
                          />
                          <template v-else>
                            {{ $t(`settings.widgetAbbr.${w.type}`, w.type) }}
                          </template>
                        </div>
                        <span
                          class="font-bold text-gray-700 text-xs leading-snug text-center truncate w-full px-1"
                        >
                          {{
                            w.type === "custom-css"
                              ? (w.data?.title || $t('settings.widgetTypes.custom-css'))
                              : $t(`settings.widgetTypes.${w.type}`, `未知组件 (${w.type})`)
                          }}
                        </span>
                      </template>
                    </div>
                    <div class="grid grid-cols-3 gap-1.5 w-full mt-1">
                      <div class="flex flex-col items-center gap-0.5">
                        <span class="text-[10px] text-gray-400 scale-90">公开</span>
                        <label class="relative inline-flex items-center cursor-pointer" title="公开"
                          ><input type="checkbox" v-model="w.isPublic" class="sr-only peer" @change="store.saveData()" />
                          <div
                            class="w-7 h-4 bg-gray-200 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-3 after:w-3 after:transition-all peer-checked:bg-blue-500"
                          ></div
                        ></label>
                      </div>
                      <div class="flex flex-col items-center gap-0.5">
                        <span class="text-[10px] text-gray-400 scale-90">启用</span>
                        <label class="relative inline-flex items-center cursor-pointer" title="启用"
                          ><input type="checkbox" v-model="w.enable" class="sr-only peer" @change="store.saveData()" />
                          <div
                            class="w-7 h-4 bg-gray-200 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-3 after:w-3 after:transition-all peer-checked:bg-green-500"
                          ></div
                        ></label>
                      </div>
                      <div class="flex flex-col items-center gap-0.5">
                        <span class="text-[10px] text-gray-400 scale-90">手机</span>
                        <label class="relative inline-flex items-center cursor-pointer" title="手机"
                          ><input
                            type="checkbox"
                            :checked="!w.hideOnMobile"
                            class="sr-only peer"
                            @change="
                              (e) => {
                                w.hideOnMobile = !(e.target as HTMLInputElement).checked;
                                store.markDirty();
                              }
                            " />
                          <div
                            class="w-7 h-4 bg-gray-200 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-3 after:w-3 after:transition-all peer-checked:bg-orange-500"
                          ></div
                        ></label>
                      </div>
                      <div v-if="w.type === 'player'" class="flex flex-col items-center gap-0.5">
                        <span class="text-[10px] text-gray-400 scale-90">自动</span>
                        <label
                          class="relative inline-flex items-center cursor-pointer"
                          title="自动播放"
                          ><input
                            type="checkbox"
                            v-model="store.appConfig.autoPlayMusic"
                            @change="store.markDirty()"
                            class="sr-only peer" />
                          <div
                            class="w-7 h-4 bg-gray-200 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-3 after:w-3 after:transition-all peer-checked:bg-blue-600"
                          ></div
                        ></label>
                      </div>
                    </div>
                  </template>
                </div>
              </template>
            </div>

            <div class="border-2 border-gray-900 rounded-xl p-4 mt-6 bg-white">
              <h4 class="text-base font-bold text-gray-900 mb-4 flex items-center gap-2">
                高级组件配置
              </h4>
              <div class="space-y-8">
                <RssSettings />
                <div class="border-t border-gray-200"></div>
                <SearchSettings />
              </div>
            </div>
          </div>

          <div v-if="activeTab === 'docker'" class="space-y-4">
            <div class="flex items-center justify-between mb-4 mr-8">
              <h4 class="text-base font-bold text-gray-900 border-l-4 border-gray-900 pl-3">
                Docker 管理 (内测中)
              </h4>
              <div
                v-if="dockerWidget || isDockerSystemEnabled"
                class="flex items-center gap-3 text-xs mr-[10px]"
              >
                <button
                  @click="exportDockerLogs"
                  :disabled="isExportingDockerLogs || !isDockerSystemEnabled"
                  class="text-gray-900 px-3 py-1 rounded-lg transition-colors font-bold disabled:opacity-60 glass-chip selectable-outline"
                >
                  {{ isExportingDockerLogs ? "导出中" : "导出日志" }}
                </button>
                <button
                  @click="checkDockerConnection"
                  :disabled="!isDockerSystemEnabled || isUpdatingDockerSystem"
                  class="text-gray-900 px-3 py-1 rounded-lg transition-colors font-bold disabled:opacity-50 glass-chip selectable-outline"
                >
                  测试连接
                </button>
              </div>
            </div>

            <div class="space-y-3 mb-6 pb-6 border-b border-gray-100">
              <div class="flex items-center justify-between">
                <div>
                  <div class="text-sm font-bold text-gray-900">{{ $t('settings.sections.dockerService') }}</div>
                  <p class="mt-1 text-xs text-gray-500">
                    {{ $t('settings.sections.dockerServiceDesc', { windowsSocket: 'npipe:////./pipe/docker_engine', linuxSocket: 'unix:///var/run/docker.sock' }) }}
                  </p>
                </div>
                <label class="relative inline-flex items-center cursor-pointer shrink-0">
                  <input
                    type="checkbox"
                    :checked="isDockerSystemEnabled"
                    :disabled="isUpdatingDockerSystem || !hasAdminAccess"
                    :aria-label="$t('settings.extraSections.dockerServiceAriaLabel')"
                    @change="(e) => toggleDockerSystemEnabled((e.target as HTMLInputElement).checked)"
                    class="sr-only peer"
                  />
                  <div
                    class="w-9 h-5 bg-gray-200 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white peer-disabled:opacity-50 after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-green-500 shrink-0"
                  ></div>
                  <span class="text-sm text-gray-700 ml-3 whitespace-nowrap">
                    {{ isUpdatingDockerSystem ? $t('settings.sections.switching') : isDockerSystemEnabled ? $t('settings.sections.enabled') : $t('settings.sections.disabled') }}
                  </span>
                </label>
              </div>

              <div
                class="rounded-xl border px-3 py-2 text-xs"
                :class="
                  isDockerSystemEnabled
                    ? 'border-emerald-200 bg-emerald-50/70 text-emerald-700'
                    : 'border-amber-200 bg-amber-50/70 text-amber-700'
                "
              >
                <p>
                  {{
                    isDockerSystemEnabled
                      ? $t('settings.sections.dockerEnabledDesc')
                      : $t('settings.sections.dockerDisabledDesc')
                  }}
                </p>
                <p class="mt-1 text-[11px] opacity-80">
                  “Docker 服务”是系统级总开关，“显示 Docker 组件”只影响首页卡片显示，“模拟数据”只影响组件展示数据来源。
                </p>
                <p v-if="!hasAdminAccess" class="mt-1 text-[11px] opacity-80">
                  {{ $t('settings.sections.dockerNote2') }}
                </p>
              </div>
            </div>

            <!-- Host Status Widget Section -->
            <div class="space-y-3 mb-6 pb-6 border-b border-gray-100">
              <div class="flex items-center justify-between">
                <span class="text-sm font-bold text-gray-900">{{ $t('settings.sections.hostStatusWidget') }}</span>
                <div class="flex items-center gap-4">
                  <div
                    v-if="systemStatusWidget && systemStatusWidget.enable"
                    class="flex items-center gap-2 animate-fade-in"
                  >
                    <div class="flex items-center gap-2">
                      <span class="text-xs text-gray-700 font-medium">{{ $t('settings.sections.publicAccessLabel') }}</span>
                      <label class="relative inline-flex items-center cursor-pointer">
                        <input
                          type="checkbox"
                          v-model="systemStatusWidget.isPublic"
                          class="sr-only peer"
                        />
                        <div
                          class="w-9 h-5 bg-gray-200 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-blue-500"
                        ></div>
                      </label>
                    </div>
                    <div class="flex items-center gap-2">
                      <span class="text-xs text-gray-700 font-medium">{{ $t('settings.sections.mobileDisplay') }}</span>
                      <label class="relative inline-flex items-center cursor-pointer">
                        <input
                          type="checkbox"
                          :checked="!systemStatusWidget.hideOnMobile"
                          @change="onMobileSystemStatusDisplayChange"
                          class="sr-only peer"
                        />
                        <div
                          class="w-9 h-5 bg-gray-200 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-orange-500"
                        ></div>
                      </label>
                    </div>
                  </div>
                  <label class="relative inline-flex items-center cursor-pointer">
                    <input
                      type="checkbox"
                      :checked="systemStatusWidget?.enable"
                      :aria-label="$t('settings.sections.enabled')"
                      @change="
                        (e) => {
                          if ((e.target as HTMLInputElement).checked) enableSystemStatusWidget();
                          else if (systemStatusWidget) {
                            systemStatusWidget.enable = false;
                            store.markDirty();
                          }
                        }
                      "
                      class="sr-only peer"
                    />
                    <div
                      class="w-9 h-5 bg-gray-200 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-green-500"
                    ></div>
                    <span class="text-sm text-gray-700 ml-3">{{ $t('settings.sections.enabled') }}</span>
                  </label>
                </div>
              </div>

              <div
                v-if="systemStatusWidget && systemStatusWidget.enable"
                class="animate-fade-in space-y-3"
              >
                <div class="flex flex-wrap items-center gap-4 border-t border-gray-100 pt-3">
                  <div class="flex items-center gap-2">
                    <span class="text-xs text-gray-500">{{ $t('settings.sections.useMockData') }}</span>
                    <label class="relative inline-flex items-center cursor-pointer">
                      <input
                        type="checkbox"
                        :checked="!!systemStatusWidget.data?.useMock"
                        @change="
                          (e) => toggleSystemStatusMock((e.target as HTMLInputElement).checked)
                        "
                        class="sr-only peer"
                      />
                      <div
                        class="w-9 h-5 bg-gray-200 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-blue-600"
                      ></div>
                    </label>
                  </div>
                </div>
                <div class="h-40 w-full max-w-sm">
                  <SystemStatusWidget :widget="systemStatusWidget" />
                </div>
              </div>
            </div>

            <div class="space-y-4">
              <!-- 未启用时的提示 -->
              <template v-if="!isDockerComponentCreated">
                <div class="flex items-center justify-between">
                  <span class="text-sm font-bold text-gray-900">{{ $t('settings.sections.containerManagement') }}</span>
                  <button
                    @click="enableDockerWidget"
                    class="px-4 py-2 bg-gray-900 text-white rounded-lg hover:bg-black transition-colors shadow-sm text-sm"
                  >
                    启用容器管理
                  </button>
                </div>
              </template>

              <!-- 已启用后的管理区 -->
              <template v-else>
                <!-- 主控制区 -->
                <div class="flex items-center justify-between">
                  <span class="text-sm font-bold text-gray-900">容器管理</span>
                  <div class="flex items-center gap-4">
                    <div class="flex items-center gap-4 animate-fade-in">
                      <div class="flex items-center gap-2">
                        <span class="text-xs text-gray-700 font-medium">公开访问</span>
                        <label class="relative inline-flex items-center cursor-pointer">
                          <input type="checkbox" v-model="dockerWidget.isPublic" class="sr-only peer" />
                          <div
                            class="w-9 h-5 bg-gray-200 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-blue-500"
                          ></div>
                        </label>
                      </div>
                      <div class="flex items-center gap-2">
                        <span class="text-xs text-gray-700 font-medium">手机端显示</span>
                        <label class="relative inline-flex items-center cursor-pointer">
                          <input type="checkbox" :checked="!dockerWidget.hideOnMobile" @change="onMobileDockerDisplayChange" class="sr-only peer" />
                          <div
                            class="w-9 h-5 bg-gray-200 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-orange-500"
                          ></div>
                        </label>
                      </div>
                    </div>
                    <label class="relative inline-flex items-center cursor-pointer">
                      <input type="checkbox" v-model="dockerWidget.enable" :aria-label="$t('settings.sections.containerManagementWidget')" class="sr-only peer" />
                      <div class="w-9 h-5 bg-gray-200 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-green-500"></div>
                      <span class="text-sm text-gray-700 ml-3">{{ $t('settings.sections.containerManagementWidget') }}</span>
                    </label>
                  </div>
                </div>

                <!-- 功能配置区 -->
                <div class="flex flex-wrap items-center gap-4 border-t border-gray-100 pt-3">
                  <!-- 模拟数据 -->
                  <div class="flex items-center gap-2">
                    <span class="text-xs text-gray-500">{{ $t('settings.sections.mockData') }}</span>
                    <label class="relative inline-flex items-center cursor-pointer">
                      <input
                        type="checkbox"
                        :checked="!!dockerWidget.data?.useMock"
                        @change="(e) => toggleDockerMock((e.target as HTMLInputElement).checked)"
                        class="sr-only peer"
                      />
                      <div
                        class="w-9 h-5 bg-gray-200 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-blue-600"
                      ></div>
                    </label>
                  </div>

                  <!-- 自动升级 -->
                  <div class="flex items-center gap-2">
                    <span class="text-xs text-gray-500">{{ $t('settings.sections.autoUpdate') }}</span>
                    <label class="relative inline-flex items-center cursor-pointer">
                      <input
                        type="checkbox"
                        :checked="!!dockerWidget.data?.autoUpdate"
                        @change="
                          (e) => {
                            if (!dockerWidget.data) dockerWidget.data = {};
                            dockerWidget.data.autoUpdate = (e.target as HTMLInputElement).checked;
                            store.markDirty();
                          }
                        "
                        class="sr-only peer"
                      />
                      <div
                        class="w-9 h-5 bg-gray-200 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-blue-600"
                      ></div>
                    </label>
                  </div>

                  <!-- 升级配置 -->
                  <div class="flex items-center gap-2">
                    <span class="text-[10px] text-gray-500">{{ $t('settings.extraSections.keepVersions') }}</span>
                    <input
                      type="number"
                      min="1"
                      max="20"
                      :disabled="!dockerWidget?.data?.autoUpdate"
                      :value="dockerWidget?.data?.autoUpdateKeepImages ?? 2"
                      @change="
                        (e) => {
                          if (!dockerWidget.data) dockerWidget.data = {};
                          dockerWidget.data.autoUpdateKeepImages = Math.max(
                            1,
                            Math.min(20, Number((e.target as HTMLInputElement).value || 2)),
                          );
                          store.markDirty();
                        }
                      "
                      class="w-16 px-2 py-1 border border-gray-200 rounded text-xs focus:border-gray-900 outline-none disabled:bg-gray-50 disabled:text-gray-400"
                    />
                    <span class="text-[10px] text-gray-500">{{ $t('settings.extraSections.unitCount') }}</span>
                  </div>

                  <div class="flex items-center gap-2">
                    <span class="text-[10px] text-gray-500">{{ $t('settings.extraSections.diskThreshold') }}</span>
                    <input
                      type="number"
                      min="0"
                      step="0.5"
                      :disabled="!dockerWidget?.data?.autoUpdate"
                      :value="dockerWidget?.data?.autoUpdateMinFreeGB ?? 5"
                      @change="
                        (e) => {
                          if (!dockerWidget.data) dockerWidget.data = {};
                          dockerWidget.data.autoUpdateMinFreeGB = Math.max(
                            0,
                            Number((e.target as HTMLInputElement).value || 5),
                          );
                          store.markDirty();
                        }
                      "
                      class="w-20 px-2 py-1 border border-gray-200 rounded text-xs focus:border-gray-900 outline-none disabled:bg-gray-50 disabled:text-gray-400"
                    />
                    <span class="text-[10px] text-gray-500">GB</span>
                  </div>

                  <!-- 内网主机 -->
                  <div class="flex items-center gap-2">
                    <span class="text-xs text-gray-700 font-medium">{{ $t('settings.extraSections.intranetHost') }}</span>
                    <input
                      :value="dockerWidget?.data?.lanHost"
                      @change="
                        (e) => {
                          if (!dockerWidget.data) dockerWidget.data = {};
                          dockerWidget.data.lanHost = (e.target as HTMLInputElement).value;
                          store.markDirty();
                        }
                      "
                      type="text"
                      :placeholder="$t('settings.extraSections.placeholderIntranetHost')"
                      class="px-2 py-1 border border-gray-200 rounded text-xs focus:border-gray-900 outline-none"
                    />
                  </div>
                </div>

                <!-- 容器预览（始终可见） -->
                <div class="border-t border-gray-100 pt-3">
                  <div
                    v-if="isDockerSystemEnabled"
                    class="h-[500px]"
                  >
                    <DockerWidget :widget="dockerWidget" :compact="true" />
                  </div>
                  <div
                    v-else
                    class="h-[160px] rounded-2xl border border-dashed border-gray-200 bg-gray-50/80 flex items-center justify-center text-center px-6"
                  >
                    <div class="text-sm text-gray-500 space-y-2">
                      <p>{{ $t('settings.extraSections.dockerServiceClosed') }}</p>
                      <p class="text-xs text-gray-400">
                        {{ $t('settings.extraSections.dockerServiceClosedDesc') }}
                      </p>
                    </div>
                  </div>
                </div>
              </template>
            </div>
          </div>

          <div v-if="activeTab === 'universal-window'" class="flatnas-handshake-signal space-y-4">
            <!-- Universal Window Widget Section -->
            <div class="flex items-center justify-between mb-4 border-b border-gray-100 pb-4">
              <div class="flex items-center gap-2">
                <h4 class="text-base font-bold text-gray-900 border-l-4 border-gray-900 pl-3">
                  {{ $t('settings.extraSections.universalWindow') }}
                </h4>
                <span class="text-xs text-gray-500 px-2 py-1 rounded-full glass-chip selected-outline">{{ $t('settings.extraSections.multiOpen') }}</span>
                <button
                  @click="addIframeWidget"
                  class="px-3 py-1.5 text-xs font-medium text-gray-900 rounded-lg transition-colors flex items-center gap-1 ml-2 glass-chip selectable-outline"
                >
                  <span class="text-base leading-none">+</span> {{ $t('settings.extraSections.addWindow') }}
                </button>
              </div>
            </div>

            <template v-for="w in store.widgets" :key="'iframe-' + w.id">
              <div
                v-if="w.type === 'iframe'"
                class="flatnas-handshake-signal flex flex-col gap-3 p-4 border border-gray-100 rounded-xl hover:shadow-md transition-all glass-card"
              >
                <div class="flex items-center justify-between">
                  <div class="flex items-center gap-4">
                    <div
                      class="w-10 h-10 rounded-full bg-white flex items-center justify-center text-sm font-medium text-gray-700 shadow-sm"
                    >
                      {{ $t('settings.widgetAbbr.iframe') }}
                    </div>
                    <div class="flex flex-col">
                      <span class="font-bold text-gray-700">{{ $t('settings.extraSections.universalWindow') }}</span>
                      <span class="text-[10px] text-gray-400 font-mono">ID: {{ w.id }}</span>
                    </div>
                  </div>
                  <div class="flex items-center gap-6">
                    <button
                      @click="removeWidget(w.id)"
                      class="text-red-400 hover:text-red-600 text-xs underline px-2"
                      :title="$t('settings.extraSections.deleteWindow')"
                    >
                      {{ $t('settings.extraSections.modalDelete') }}
                    </button>
                    <div class="flex flex-col items-end gap-1">
                      <span class="text-[10px] text-gray-400 font-medium">{{ $t('settings.sections.publicAccess') }}</span
                      ><label class="relative inline-flex items-center cursor-pointer"
                        ><input
                          type="checkbox"
                          v-model="w.isPublic"
                          class="sr-only peer"
                          @change="store.markDirty()" />
                        <div
                          class="w-9 h-5 bg-gray-200 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-blue-500"
                        ></div
                      ></label>
                    </div>
                    <div class="flex flex-col items-end gap-1">
                      <span class="text-[10px] text-gray-400 font-medium">{{ $t('settings.sections.mobileAccess') }}</span
                      ><label class="relative inline-flex items-center cursor-pointer"
                        ><input
                          type="checkbox"
                          :checked="!w.hideOnMobile"
                          class="sr-only peer"
                          @change="
                            (e) => {
                              w.hideOnMobile = !(e.target as HTMLInputElement).checked;
                              store.markDirty();
                            }
                          " />
                        <div
                          class="w-9 h-5 bg-gray-200 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-orange-500"
                        ></div
                      ></label>
                    </div>
                    <div class="flex flex-col items-end gap-1">
                      <span class="text-[10px] text-gray-400 font-medium">{{ $t('settings.sections.enabled') }}</span
                      ><label class="relative inline-flex items-center cursor-pointer"
                        ><input
                          type="checkbox"
                          v-model="w.enable"
                          class="sr-only peer"
                          @change="store.markDirty()" />
                        <div
                          class="w-9 h-5 bg-gray-200 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-green-500"
                        ></div
                      ></label>
                    </div>
                  </div>
                </div>
                <div class="w-full bg-white/60 p-3 rounded-lg border border-gray-100 space-y-3">
                  <div>
                    <div class="flex justify-between items-center mb-1">
                      <label class="block text-xs font-bold text-gray-600"
                        >{{ $t('settings.extraSections.publicNetworkUrl') }}</label
                      >
                      <ProxyToggle v-model="w.data.useProxy" @update:model-value="store.markDirty()" />
                    </div>
                    <div class="flex gap-2">
                      <input
                        :value="tempInputs[`${w.id}-url`] ?? w.data.url"
                        @input="
                          (e: Event) =>
                            updateTempInput(`${w.id}-url`, (e.target as HTMLInputElement).value)
                        "
                        type="url"
                        :placeholder="$t('settings.extraSections.placeholderExampleUrl')"
                        class="flex-1 px-3 py-2 border border-gray-200 rounded-lg text-xs focus:border-gray-900 outline-none"
                        @keydown.enter="confirmTempInput(w, 'url', `${w.id}-url`)"
                      />
                      <button
                        @click="confirmTempInput(w, 'url', `${w.id}-url`)"
                        class="px-3 py-2 bg-yellow-500 text-white rounded-lg text-xs font-bold hover:bg-yellow-600 transition-colors whitespace-nowrap"
                      >
                        {{ $t('settings.extraSections.modalConfirm') }}
                      </button>
                    </div>
                  </div>
                  <div>
                    <label
                      class="block text-xs font-bold text-gray-600 mb-1 flex items-center gap-1"
                    >
                      <span>{{ $t('settings.extraSections.lanUrlLabel') }}</span>
                      <span class="text-[10px] font-normal text-gray-400 px-1.5 rounded glass-chip selected-outline"
                        >{{ $t('settings.extraSections.lanPriority') }}</span
                      >
                    </label>
                    <div class="flex gap-2">
                      <input
                        :value="tempInputs[`${w.id}-lanUrl`] ?? w.data.lanUrl"
                        @input="
                          (e: Event) =>
                            updateTempInput(`${w.id}-lanUrl`, (e.target as HTMLInputElement).value)
                        "
                        type="url"
                        :placeholder="$t('settings.extraSections.placeholderLanUrl')"
                        class="flex-1 px-3 py-2 border border-gray-200 rounded-lg text-xs focus:border-gray-900 outline-none"
                        @keydown.enter="confirmTempInput(w, 'lanUrl', `${w.id}-lanUrl`)"
                      />
                      <button
                        @click="confirmTempInput(w, 'lanUrl', `${w.id}-lanUrl`)"
                        class="px-3 py-2 bg-yellow-500 text-white rounded-lg text-xs font-bold hover:bg-yellow-600 transition-colors whitespace-nowrap"
                      >
                        确定
                      </button>
                    </div>
                  </div>
                  <p class="text-[10px] text-gray-500 mt-1">
                    点击<a
                      href="/flatnas-helper.zip"
                      download="flatnas-helper.zip"
                      target="_blank"
                      class="text-blue-500 underline mx-1"
                      >{{ $t('settings.extraSections.downloadBrowserPlugin') }}</a
                    >{{ $t('settings.extraSections.removeRestriction') }}
                  </p>
                  <p class="text-[10px] text-gray-400 mt-1">
                    {{ $t('settings.extraSections.networkSwitchHint') }}
                  </p>
                </div>
              </div>
            </template>

            <!-- Amap Weather Widget Section -->
            <div class="flex items-center justify-between mb-4 border-b border-gray-100 pb-4 mt-8">
              <div class="flex items-center gap-2">
                <h4 class="text-base font-bold text-gray-900 border-l-4 border-gray-900 pl-3">
                  {{ $t('settings.extraSections.amapWeatherFull') }}
                </h4>
                <span class="text-xs text-gray-500 px-2 py-1 rounded-full glass-chip selected-outline">{{ $t('settings.extraSections.multiOpen') }}</span>
                <button
                  @click="addAmapWeatherWidget"
                  class="px-3 py-1.5 text-xs font-medium text-gray-900 rounded-lg transition-colors flex items-center gap-1 ml-2 glass-chip selectable-outline"
                >
                  <span class="text-base leading-none">+</span> {{ $t('settings.extraSections.addWeatherWidget') }}
                </button>
              </div>
            </div>

            <template v-for="w in store.widgets" :key="'amap-' + w.id">
              <div
                v-if="w.type === 'amap-weather'"
                class="flatnas-handshake-signal flex flex-col gap-3 p-4 border border-gray-100 rounded-xl bg-gray-50 hover:bg-white hover:shadow-md transition-all"
              >
                <div class="flex items-center justify-between">
                  <div class="flex items-center gap-4">
                    <div
                      class="w-10 h-10 rounded-full bg-white flex items-center justify-center text-sm font-medium text-gray-700 shadow-sm"
                    >
                      {{ $t('settings.widgetAbbr.amap-weather') }}
                    </div>
                    <div class="flex flex-col">
                      <span class="font-bold text-gray-700">{{ $t('settings.extraSections.amapWeatherFull') }}</span>
                      <span class="text-[10px] text-gray-400 font-mono">ID: {{ w.id }}</span>
                    </div>
                  </div>
                  <div class="flex items-center gap-6">
                    <button
                      @click="removeWidget(w.id)"
                      class="text-gray-400 hover:text-gray-900 text-xs underline px-2"
                      :title="$t('settings.extraSections.deleteComponent')"
                    >
                      删除
                    </button>
                    <div class="flex flex-col items-end gap-1">
                      <span class="text-[10px] text-gray-400 font-medium">{{ $t('settings.sections.publicAccess') }}</span
                      ><label class="relative inline-flex items-center cursor-pointer"
                        ><input
                          type="checkbox"
                          v-model="w.isPublic"
                          class="sr-only peer"
                          @change="store.markDirty()" />
                        <div
                          class="w-9 h-5 bg-gray-200 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-blue-500"
                        ></div
                      ></label>
                    </div>
                    <div class="flex flex-col items-end gap-1">
                      <span class="text-[10px] text-gray-400 font-medium">{{ $t('settings.sections.mobileAccess') }}</span
                      ><label class="relative inline-flex items-center cursor-pointer"
                        ><input
                          type="checkbox"
                          :checked="!w.hideOnMobile"
                          class="sr-only peer"
                          @change="
                            (e) => {
                              w.hideOnMobile = !(e.target as HTMLInputElement).checked;
                              store.markDirty();
                            }
                          " />
                        <div
                          class="w-9 h-5 bg-gray-200 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-orange-500"
                        ></div
                      ></label>
                    </div>
                    <div class="flex flex-col items-end gap-1">
                      <span class="text-[10px] text-gray-400 font-medium">{{ $t('settings.sections.enabled') }}</span
                      ><label class="relative inline-flex items-center cursor-pointer"
                        ><input
                          type="checkbox"
                          v-model="w.enable"
                          class="sr-only peer"
                          @change="store.markDirty()" />
                        <div
                          class="w-9 h-5 bg-gray-200 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-green-500"
                        ></div
                      ></label>
                    </div>
                  </div>
                </div>
              </div>
            </template>

            <!-- Countdown Widget Section -->
            <div class="flex items-center justify-between mb-4 border-b border-gray-100 pb-4 mt-8">
              <div class="flex items-center gap-2">
                <h4 class="text-base font-bold text-gray-900 border-l-4 border-gray-900 pl-3">
                  {{ $t('settings.extraSections.countdownSection') }}
                </h4>
                <span class="text-xs text-gray-500 px-2 py-1 rounded-full glass-chip selected-outline">{{ $t('settings.extraSections.multiOpen') }}</span>
                <button
                  @click="addCountdownWidget"
                  class="px-3 py-1.5 text-xs font-medium text-gray-900 rounded-lg transition-colors flex items-center gap-1 ml-2 glass-chip selectable-outline"
                >
                  <span class="text-base leading-none">+</span> {{ $t('settings.extraSections.addCountdown') }}
                </button>
              </div>
            </div>

            <template v-for="w in store.widgets" :key="'cd-' + w.id">
              <div
                v-if="w.type === 'countdown'"
                class="flatnas-handshake-signal flex flex-col gap-3 p-4 border border-gray-100 rounded-xl bg-gray-50 hover:bg-white hover:shadow-md transition-all"
              >
                <div class="flex items-center justify-between">
                  <div class="flex items-center gap-4">
                    <div
                      class="w-10 h-10 rounded-full bg-white flex items-center justify-center text-sm font-medium text-gray-700 shadow-sm">
                      {{ $t('settings.widgetAbbr.countdown') }}
                    </div>
                    <div class="flex flex-col">
                      <span class="font-bold text-gray-700">{{ $t('settings.extraSections.countdownSection') }}</span>
                      <span class="text-[10px] text-gray-400 font-mono">ID: {{ w.id }}</span>
                    </div>
                  </div>
                  <div class="flex items-center gap-6">
                    <button
                      @click="removeWidget(w.id)"
                      class="text-gray-400 hover:text-gray-900 text-xs underline px-2"
                      :title="$t('settings.extraSections.deleteComponent')"
                    >
                      删除
                    </button>
                    <div class="flex flex-col items-end gap-1">
                      <span class="text-[10px] text-gray-400 font-medium">{{ $t('settings.sections.publicAccess') }}</span
                      ><label class="relative inline-flex items-center cursor-pointer"
                        ><input
                          type="checkbox"
                          v-model="w.isPublic"
                          class="sr-only peer"
                          @change="store.markDirty()" />
                        <div
                          class="w-9 h-5 bg-gray-200 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-blue-500"
                        ></div
                      ></label>
                    </div>
                    <div class="flex flex-col items-end gap-1">
                      <span class="text-[10px] text-gray-400 font-medium">{{ $t('settings.sections.mobileAccess') }}</span
                      ><label class="relative inline-flex items-center cursor-pointer"
                        ><input
                          type="checkbox"
                          :checked="!w.hideOnMobile"
                          class="sr-only peer"
                          @change="
                            (e) => {
                              w.hideOnMobile = !(e.target as HTMLInputElement).checked;
                              store.markDirty();
                            }
                          " />
                        <div
                          class="w-9 h-5 bg-gray-200 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-orange-500"
                        ></div
                      ></label>
                    </div>
                    <div class="flex flex-col items-end gap-1">
                      <span class="text-[10px] text-gray-400 font-medium">{{ $t('settings.sections.enabled') }}</span
                      ><label class="relative inline-flex items-center cursor-pointer"
                        ><input
                          type="checkbox"
                          v-model="w.enable"
                          class="sr-only peer"
                          @change="store.markDirty()" />
                        <div
                          class="w-9 h-5 bg-gray-200 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-green-500"
                        ></div
                      ></label>
                    </div>
                  </div>
                </div>
                <div class="w-full bg-white/60 p-3 rounded-lg border border-gray-100 space-y-3">
                  <div>
                    <label class="block text-xs font-bold text-gray-600 mb-1">标题</label>
                    <input
                      v-model="w.data.title"
                      type="text"
                      :placeholder="$t('settings.extraSections.placeholderCountdownTitle')"
                      class="w-full px-3 py-2 border border-gray-200 rounded-lg text-xs focus:border-gray-900 outline-none"
                    />
                  </div>
                  <div>
                    <label class="block text-xs font-bold text-gray-600 mb-1">目标时间</label>
                    <input
                      v-model="w.data.targetDate"
                      type="datetime-local"
                      class="w-full px-3 py-2 border border-gray-200 rounded-lg text-xs focus:border-gray-900 outline-none"
                    />
                  </div>
                </div>
              </div>
            </template>

            <!-- CountUp Widget Section -->
            <div class="flex items-center justify-between mb-4 border-b border-gray-100 pb-4 mt-8">
              <div class="flex items-center gap-2">
                <h4 class="text-base font-bold text-gray-900 border-l-4 border-gray-900 pl-3">
                  {{ $t('settings.extraSections.countupSection') }}
                </h4>
                <span class="text-xs text-gray-500 px-2 py-1 rounded-full glass-chip selected-outline">{{ $t('settings.extraSections.multiOpen') }}</span>
                <button
                  @click="addCountUpWidget"
                  class="px-3 py-1.5 text-xs font-medium text-gray-900 rounded-lg transition-colors flex items-center gap-1 ml-2 glass-chip selectable-outline"
                >
                  <span class="text-base leading-none">+</span> {{ $t('settings.extraSections.addCountup') }}
                </button>
              </div>
            </div>

            <template v-for="w in store.widgets" :key="'cup-' + w.id">
              <div
                v-if="w.type === 'countup'"
                class="flatnas-handshake-signal flex flex-col gap-3 p-4 border border-gray-100 rounded-xl bg-gray-50 hover:bg-white hover:shadow-md transition-all"
              >
                <div class="flex items-center justify-between">
                  <div class="flex items-center gap-4">
                    <div
                      class="w-10 h-10 rounded-full bg-white flex items-center justify-center text-sm font-medium text-gray-700 shadow-sm"
                    >
                      正
                    </div>
                    <div class="flex flex-col">
                      <span class="font-bold text-gray-700">{{ $t('settings.extraSections.countupSection') }}</span>
                      <span class="text-[10px] text-gray-400 font-mono">ID: {{ w.id }}</span>
                    </div>
                  </div>
                  <div class="flex items-center gap-6">
                    <button
                      @click="removeWidget(w.id)"
                      class="text-gray-400 hover:text-gray-900 text-xs underline px-2"
                      :title="$t('settings.extraSections.deleteComponent')"
                    >
                      删除
                    </button>
                    <div class="flex flex-col items-end gap-1">
                      <span class="text-[10px] text-gray-400 font-medium">{{ $t('settings.sections.publicAccess') }}</span
                      ><label class="relative inline-flex items-center cursor-pointer"
                        ><input
                          type="checkbox"
                          v-model="w.isPublic"
                          class="sr-only peer"
                          @change="store.markDirty()" />
                        <div
                          class="w-9 h-5 bg-gray-200 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-blue-500"
                        ></div
                      ></label>
                    </div>
                    <div class="flex flex-col items-end gap-1">
                      <span class="text-[10px] text-gray-400 font-medium">{{ $t('settings.sections.mobileAccess') }}</span
                      ><label class="relative inline-flex items-center cursor-pointer"
                        ><input
                          type="checkbox"
                          :checked="!w.hideOnMobile"
                          class="sr-only peer"
                          @change="
                            (e) => {
                              w.hideOnMobile = !(e.target as HTMLInputElement).checked;
                              store.markDirty();
                            }
                          " />
                        <div
                          class="w-9 h-5 bg-gray-200 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-orange-500"
                        ></div
                      ></label>
                    </div>
                    <div class="flex flex-col items-end gap-1">
                      <span class="text-[10px] text-gray-400 font-medium">{{ $t('settings.sections.enabled') }}</span
                      ><label class="relative inline-flex items-center cursor-pointer"
                        ><input
                          type="checkbox"
                          v-model="w.enable"
                          class="sr-only peer"
                          @change="store.markDirty()" />
                        <div
                          class="w-9 h-5 bg-gray-200 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-green-500"
                        ></div
                      ></label>
                    </div>
                  </div>
                </div>
                <div class="w-full bg-white/60 p-3 rounded-lg border border-gray-100 space-y-3">
                  <div>
                    <label class="block text-xs font-bold text-gray-600 mb-1">标题</label>
                    <input
                      v-model="w.data.title"
                      type="text"
                      :placeholder="$t('settings.extraSections.placeholderCountupTitle')"
                      class="w-full px-3 py-2 border border-gray-200 rounded-lg text-xs focus:border-gray-900 outline-none"
                    />
                  </div>
                  <div>
                    <label class="block text-xs font-bold text-gray-600 mb-1">开始时间</label>
                    <input
                      v-model="w.data.startTime"
                      type="datetime-local"
                      step="1"
                      class="w-full px-3 py-2 border border-gray-200 rounded-lg text-xs focus:border-gray-900 outline-none"
                    />
                  </div>
                </div>
              </div>
            </template>
          </div>

          <div v-if="activeTab === 'network'" class="p-4 space-y-4">
            <div class="flex items-center gap-3 mb-4">
              <h4 class="text-base font-bold text-gray-900 border-l-4 border-gray-900 pl-3">
                {{ $t('settings.extraSections.networkDetectionSettings') }}
              </h4>
            </div>

            <div class="bg-gray-50 border border-gray-100 rounded-xl p-4 space-y-3">
              <div class="flex items-start gap-2">
                <span
                  class="text-xs text-gray-500 font-medium border border-gray-200 rounded px-1.5 py-0.5 mt-0.5"
                  >注</span
                >
                <p class="text-xs text-gray-600 leading-relaxed">
                  {{ $t('settings.extraSections.domainWhitelistHint') }}
                </p>
              </div>

              <div class="space-y-2">
                <label class="block text-sm font-medium text-gray-700">{{ $t('settings.extraSections.domainWhitelist') }}</label>
                <textarea
                  v-model="store.appConfig.internalDomains"
                  @change="store.markDirty()"
                  rows="5"
                  class="w-full px-3 py-2 border border-gray-200 rounded-lg text-xs focus:border-gray-900 outline-none font-mono"
                  :placeholder="$t('settings.extraSections.placeholderDomainWhitelist')"
                ></textarea>
              </div>
            </div>

            <div class="bg-gray-50 border border-gray-100 rounded-xl p-4">
              <h5 class="text-sm font-medium text-gray-700 mb-3">{{ $t('settings.extraSections.whitelistLatencyMode') }}</h5>
              <div class="flex items-center gap-3 mb-3">
                <button
                  type="button"
                  @click="toggleWhitelistLatency"
                  class="px-3 py-1.5 rounded-lg text-xs font-bold transition-colors border whitespace-nowrap"
                  :class="
                    whitelistLatencyEnabled
                      ? 'bg-blue-50 text-blue-600 border-blue-200 hover:bg-blue-100'
                      : 'bg-white text-gray-700 border-gray-200 hover:bg-gray-50'
                  "
                >
                  {{ whitelistLatencyEnabled ? $t('settings.extraSections.whitelistLatencyEnabled') : $t('settings.extraSections.enableWhitelistLatency') }}
                </button>
                <span class="text-[11px] text-gray-500">
                  {{ whitelistLatencyEnabled ? $t('settings.extraSections.whitelistLatencyDesc') : $t('settings.extraSections.whitelistLatencyDisabledDesc') }}
                </span>
              </div>
              <div v-if="whitelistLatencyEnabled" class="flex items-center gap-2">
                <input
                  :value="latencyThresholdDraft"
                  inputmode="numeric"
                  @input="onLatencyThresholdInput"
                  @blur="onLatencyThresholdBlur"
                  @keydown.enter.prevent="applyLatencyThreshold"
                  placeholder="20–30000"
                  class="w-32 px-3 py-2 border rounded-lg text-xs outline-none font-mono focus:border-gray-900"
                  :class="
                    latencyThresholdTouched && !latencyThresholdValidation.ok
                      ? 'border-red-300'
                      : 'border-gray-200'
                  "
                />
                <span class="text-xs text-gray-500">ms</span>
                <button
                  type="button"
                  @click="applyLatencyThreshold"
                  :disabled="!latencyThresholdValidation.ok"
                  class="px-3 py-2 rounded-lg text-xs font-bold transition-colors whitespace-nowrap"
                  :class="
                    latencyThresholdValidation.ok
                      ? 'bg-blue-600 text-white hover:bg-blue-500'
                      : 'bg-gray-200 text-gray-500 cursor-not-allowed'
                  "
                >
                  {{ $t('settings.extraSections.confirmApplyDefaults') }}
                </button>
                <button
                  type="button"
                  @click="resetLatencyThreshold"
                  class="px-3 py-2 bg-white text-gray-600 border border-gray-200 rounded-lg text-xs font-bold hover:bg-gray-50 transition-colors whitespace-nowrap"
                >
                  {{ $t('settings.extraSections.resetToDefault') }}
                </button>
                <div class="text-[10px] text-gray-400">
                  {{ $t('settings.extraSections.defaultLatency', { ms: DEFAULT_LATENCY_THRESHOLD_MS }) }}
                </div>
              </div>
              <p
                v-if="latencyThresholdTouched && !latencyThresholdValidation.ok"
                class="mt-2 text-[11px] text-red-600"
              >
                {{ latencyThresholdValidation.error }}
              </p>
              <p v-else-if="latencyThresholdAppliedToast" class="mt-2 text-[11px] text-green-600">
                {{ latencyThresholdAppliedToast }}
              </p>
              <p v-else class="mt-2 text-[11px] text-gray-500">
                {{ $t('settings.extraSections.latencyThresholdDesc', { ms: DEFAULT_LATENCY_THRESHOLD_MS }) }}
              </p>
            </div>
          </div>

          <div v-if="activeTab === 'lucky-stun'" class="p-4 space-y-4">
            <div class="flex items-center gap-2 mb-4">
              <h4 class="text-base font-bold text-gray-900 border-l-4 border-gray-900 pl-3">
                {{ $t('settings.tabs.luckyStun') }}
              </h4>
            </div>

            <div class="bg-gray-50 border border-gray-100 rounded-xl p-4 mb-6 space-y-3">
              <div class="flex items-center justify-between gap-3">
                <div>
                  <h4 class="text-base font-bold text-gray-900">{{ $t('settings.extraSections.browserHelperComms') }}</h4>
                  <p class="text-xs text-gray-500 mt-1">
                    {{ $t('settings.extraSections.browserHelperDesc') }}
                  </p>
                </div>
              </div>

              <template v-if="browserHelperHandshakeLink">
                <a
                  :href="browserHelperHandshakeLink"
                  :data-flatnas-helper-link="browserHelperHandshakeLink"
                  target="_blank"
                  rel="noopener noreferrer"
                  class="block w-full px-3 py-2 rounded-lg border border-gray-200 bg-white text-[11px] text-blue-600 break-all hover:border-blue-300 transition-colors"
                >
                  {{ browserHelperHandshakeLink }}
                </a>
                <div class="flex flex-wrap items-center gap-2">
                  <button
                    @click="copyBrowserHelperHandshakeLink"
                    class="px-3 py-1.5 bg-gray-900 hover:bg-black text-white text-xs rounded-lg transition-colors"
                  >
                    复制握手链接
                  </button>
                  <button
                    @click="openBrowserHelperHandshakeLink"
                    class="px-3 py-1.5 bg-white border border-gray-200 hover:border-gray-300 text-gray-700 text-xs rounded-lg transition-colors"
                  >
                    打开握手页
                  </button>
                  <a
                    href="/flatnas-helper.zip"
                    download="flatnas-helper.zip"
                    class="px-3 py-1.5 bg-blue-600 hover:bg-blue-700 text-white text-xs rounded-lg transition-colors inline-block"
                  >
                    下载浏览器助手
                  </a>
                </div>
                <p class="text-[11px] text-gray-500">
                  {{ $t('settings.extraSections.helperNotDetected') }}
                </p>
              </template>
              <p v-else class="text-[11px] text-amber-600">
                {{ $t('settings.extraSections.noLoginToken') }}
              </p>
            </div>


            <!-- Music Widget Settings -->
            <div id="music-settings" class="bg-gray-50 border border-gray-100 rounded-xl p-4 mb-6">
              <div class="flex items-center justify-between mb-4">
                <h4 class="text-base font-bold text-gray-900">🎵 音乐（LX 引擎）</h4>

              </div>

              <div class="space-y-4 animate-fade-in">
                <p class="text-xs text-gray-500">音乐引擎与云栖阁在同一个项目里运行：搜歌走引擎曲库，一次搜多家、界面上不分平台；播放地址只由下面导入的「音源脚本」解析。下载的歌会存到 NAS 的音乐目录。</p>

                <!-- 代理与默认值 -->
                <div class="bg-white border border-gray-200 rounded-lg p-3 space-y-3">
                  <div class="flex items-center justify-between">
                    <span class="text-sm font-bold text-gray-800">代理（音乐平台需要）</span>
                    <input type="checkbox" v-model="lxConfig.proxyEnabled" class="w-4 h-4 accent-gray-900" />
                  </div>
                  <div v-if="lxConfig.proxyEnabled" class="grid grid-cols-3 gap-2">
                    <input v-model="lxConfig.proxyHost" placeholder="代理主机" class="col-span-2 px-3 py-2 border border-gray-200 rounded-lg text-sm outline-none focus:border-gray-900" />
                    <input v-model.number="lxConfig.proxyPort" type="number" placeholder="端口" class="px-3 py-2 border border-gray-200 rounded-lg text-sm outline-none focus:border-gray-900" />
                  </div>
                  <div class="grid grid-cols-3 gap-2">
                    <div class="col-span-1">
                      <label class="block text-xs font-bold text-gray-600 mb-1">下载目录</label>
                      <input v-model="lxConfig.downloadDir" placeholder="/downloads" class="w-full px-2 py-2 border border-gray-200 rounded-lg text-sm outline-none" />
                    </div>
                    <div class="col-span-2 flex items-end">
                      <span class="text-[11px] text-gray-500 leading-snug">支持任意磁盘：填 vol1/... 或 vol2/... 开头的路径（如 vol2/1000/音乐）下到对应盘；只填名字（如「华语」）= 音乐文件夹 /vol1/1000/music-downloads 下的子目录。</span>
                    </div>
                    <div>
                      <label class="block text-xs font-bold text-gray-600 mb-1">默认音质</label>
                      <select v-model="lxConfig.defaultQuality" class="w-full px-2 py-2 border border-gray-200 rounded-lg text-sm outline-none">
                        <option value="128k">128k</option>
                        <option value="320k">320k</option>
                        <option value="flac">FLAC</option>
                        <option value="flac24bit">FLAC 24bit</option>
                      </select>
                    </div>
                    <div>
                      <label class="block text-xs font-bold text-gray-600 mb-1">默认模式</label>
                      <select v-model="lxConfig.defaultMode" class="w-full px-2 py-2 border border-gray-200 rounded-lg text-sm outline-none">
                        <option value="play">只播放</option>
                        <option value="playDownload">边播边下</option>
                        <option value="download">只下载</option>
                      </select>
                    </div>
                  </div>
                  <label class="flex items-center justify-between cursor-pointer">
                    <span class="text-sm text-gray-700">下载歌曲时同时下载歌词（存同名 .lrc 文件，歌单里播放能看歌词）</span>
                    <input type="checkbox" v-model="lxConfig.lyricDownload" class="w-4 h-4 accent-gray-900 shrink-0 ml-3" />
                  </label>
                  <button @click="saveLxConfig" :disabled="lxSaving" class="w-full bg-gray-900 text-white px-4 py-2 rounded-lg text-sm font-bold hover:bg-gray-800 disabled:opacity-50">
                    {{ lxSaving ? "保存中…" : "保存音乐设置" }}
                  </button>
                  <div v-if="lxSaveMsg" class="text-xs" :class="lxSaveOk ? 'text-green-600' : 'text-red-500'">{{ lxSaveMsg }}</div>
                </div>

                <!-- 音源脚本 -->
                <div class="bg-white border border-gray-200 rounded-lg p-3 space-y-3">
                  <div class="flex items-center justify-between">
                    <span class="text-sm font-bold text-gray-800">音源脚本（决定能不能解析出播放地址）</span>
                    <button @click="loadLxSources" class="text-xs text-gray-500 hover:text-gray-900">刷新</button>
                  </div>
                  <div v-if="!lxSources.length" class="text-xs text-gray-400">还没有导入音源。把下面任意一个点「导入」即可，多个都导入会自动轮换使用。</div>
                  <div v-for="s in lxSources" :key="s.id" class="flex items-center gap-2 text-xs border border-gray-100 rounded-lg px-2 py-2">
                    <span class="flex-1 min-w-0">
                      <span class="font-bold text-gray-800">{{ s.name || s.id }}</span>
                      <span class="text-gray-400 ml-1">{{ s.version || "" }}</span>
                      <span class="block text-gray-400 truncate">{{ s.ok ? '支持：' + Object.keys(s.sources || {}).join('、') : '加载失败：' + (s.error || '') }}</span>
                    </span>
                    <span :class="s.ok ? 'text-green-600' : 'text-red-500'">{{ s.ok ? "可用" : "失败" }}</span>
                    <button @click="removeLxSource(s.id)" class="px-2 py-1 rounded bg-red-50 text-red-600 border border-red-200 font-bold">删除</button>
                  </div>
                  <div class="flex gap-2">
                    <input v-model="lxSourceUrl" placeholder="粘贴音源脚本的链接（.js）" class="flex-1 min-w-0 px-3 py-2 border border-gray-200 rounded-lg text-sm outline-none focus:border-gray-900" />
                    <button @click="importLxSource()" :disabled="lxImporting || !lxSourceUrl" class="px-3 py-2 rounded-lg bg-gray-900 text-white text-sm font-bold disabled:opacity-50">导入</button>
                  </div>
                  <div>
                    <div class="text-xs font-bold text-gray-600 mb-1">常用音源（点导入）</div>
                    <div class="flex flex-wrap gap-1.5">
                      <button v-for="p in lxPresetSources" :key="p.name" @click="importLxSource(p.url)" :disabled="lxImporting" class="px-2 py-1 rounded-full bg-gray-100 hover:bg-gray-200 text-xs text-gray-700 disabled:opacity-50">{{ p.name }}</button>
                    </div>
                  </div>
                  <div v-if="lxImportMsg" class="text-xs text-gray-500">{{ lxImportMsg }}</div>
                </div>
              </div>
            </div>

            <!-- VPS Monitor Settings -->
            <div id="vps-settings" class="bg-gray-50 border border-gray-100 rounded-xl p-4 mb-6">
              <div class="flex items-center justify-between mb-4">
                <h4 class="text-base font-bold text-gray-900">🖥️ VPS 监控（1Panel 接口）</h4>
                <button v-if="!hasVpsWidget" @click="enableVpsMonitorWidget" class="px-3 py-1.5 bg-gray-900 text-white text-xs rounded-lg font-bold hover:bg-gray-800">把组件加到首页</button>
                <span v-else class="text-xs text-green-600 font-bold">组件已在首页</span>
              </div>
              <div class="space-y-3">
                <div class="bg-amber-50 border border-amber-200 rounded-lg p-3 text-xs text-amber-800 leading-relaxed">
                  <div class="font-bold mb-1">⚠️ 新服务器要先装日志服务（只需一次）</div>
                  想在组件里看到「SSH 爆破次数」，服务器上必须有 SSH 日志文件；新系统默认没装。打开该服务器 1Panel 的「终端」，运行这条命令（约半分钟）：
                  <code class="block mt-1.5 bg-gray-900 text-green-300 rounded px-2 py-1.5 font-mono text-[11px] break-all select-all">apt update &amp;&amp; apt install rsyslog -y &amp;&amp; systemctl enable --now rsyslog</code>
                  <div class="mt-1.5">装完不用再管，组件几分钟内自动开始显示爆破数据；没装的服务器只是少这一项数据，其它监控不受影响。</div>
                </div>
                <p class="text-xs text-gray-500">数据从每台 VPS 的 1Panel 面板接口采集，不用 SSH、不用存机器密码：打开 1Panel → 设置 → API 接口，开启并生成密钥后填到这里。每 20 秒自动刷新：CPU、内存、磁盘、容器（含 sing-box）状态、网络速率。海外面板地址国内直连常被掐，默认走 NAS 的 sing-box 代理访问。</p>
                <div v-for="n in vpsNodes" :key="n.id" class="bg-white border border-gray-200 rounded-lg p-3 space-y-2">
                  <div class="grid grid-cols-2 gap-2">
                    <input v-model="n.name" placeholder="名称，如：美国 VPS" class="px-3 py-2 border border-gray-200 rounded-lg text-sm outline-none focus:border-gray-900" />
                    <input v-model="n.baseUrl" placeholder="面板地址，如 https://panel.xxx.cc:端口" class="px-3 py-2 border border-gray-200 rounded-lg text-sm outline-none focus:border-gray-900" />
                  </div>
                  <div class="flex gap-2">
                    <input v-model="n.apiKey" type="password" :placeholder="n.hasKey ? 'API 密钥已保存（留空不改）' : '粘贴 1Panel 的 API 密钥'" class="flex-1 min-w-0 px-3 py-2 border border-gray-200 rounded-lg text-sm outline-none focus:border-gray-900" />
                    <label class="flex items-center gap-1.5 text-xs text-gray-600 shrink-0">
                      <input type="checkbox" v-model="n.useProxy" class="w-4 h-4 accent-gray-900" /> 走代理
                    </label>
                    <button @click="testVpsNode(n)" :disabled="vpsTestingId === n.id" class="px-3 py-2 rounded-lg bg-white border border-gray-300 text-sm font-bold text-gray-700 disabled:opacity-50 shrink-0">{{ vpsTestingId === n.id ? "测试中…" : "测试" }}</button>
                    <button @click="deleteVpsNode(n)" class="px-3 py-2 rounded-lg bg-red-50 text-red-600 border border-red-200 text-sm font-bold shrink-0">删除</button>
                  </div>
                  <div v-if="vpsTestMsg[n.id]" class="text-xs" :class="vpsTestMsg[n.id].startsWith('连接成功') ? 'text-green-600' : 'text-red-500'">{{ vpsTestMsg[n.id] }}</div>
                </div>
                <div v-if="!vpsNodes.length" class="text-xs text-gray-400">还没有节点，点下面「添加节点」开始。</div>
                <div class="bg-white border border-gray-200 rounded-lg p-3">
                  <div class="flex items-center gap-2 flex-wrap">
                    <span class="text-sm font-bold text-gray-700">探测周期</span>
                    <input v-model.number="vpsIntervalSec" type="number" min="10" max="600" step="5" class="w-20 px-2 py-1.5 border border-gray-200 rounded-lg text-sm outline-none focus:border-gray-900" />
                    <span class="text-xs text-gray-500">秒（10–600）</span>
                    <button v-for="p in [10, 20, 30, 60, 120, 300]" :key="p" @click="vpsIntervalSec = p" class="px-2 py-1 rounded-md border text-xs font-bold" :class="vpsIntervalSec === p ? 'bg-gray-900 text-white border-gray-900' : 'bg-white text-gray-600 border-gray-300 hover:border-gray-500'">{{ p }}秒</button>
                  </div>
                  <p class="text-xs text-gray-400 mt-1.5">每隔多久把所有 VPS 采集一遍，首页组件跟着同节奏刷新。改完点下面「保存节点设置」生效。太频繁会给面板和代理添负担，一般 20–60 秒就够。</p>
                </div>
                <div class="flex gap-2">
                  <button @click="addVpsNodeRow" class="px-3 py-2 rounded-lg bg-white border border-gray-300 text-sm font-bold text-gray-700">＋ 添加节点</button>
                  <button @click="saveVpsNodes" :disabled="vpsSaving" class="flex-1 bg-gray-900 text-white px-4 py-2 rounded-lg text-sm font-bold hover:bg-gray-800 disabled:opacity-50">{{ vpsSaving ? "保存中…" : "保存节点设置" }}</button>
                </div>
                <div v-if="vpsSaveMsg" class="text-xs" :class="vpsSaveOk ? 'text-green-600' : 'text-red-500'">{{ vpsSaveMsg }}</div>
              </div>
            </div>
            <!-- Custom CSS Section -->
            <div class="bg-gray-50 border border-gray-100 rounded-xl p-4 mb-6">
              <h4 class="text-base font-bold mb-4 text-gray-900">{{ $t('settings.extraSections.customCss') }}</h4>
              <div>
                <ScriptManager
                  v-if="store.appConfig.customCssList"
                  v-model="store.appConfig.customCssList"
                  type="css"
                  placeholder="/* 输入自定义 CSS 代码 */
.card-item {
  border-radius: 20px;
}"
                  @change="store.updateCustomScripts()"
                />
                <div class="text-xs text-gray-500 mt-2">
                  {{ $t('settings.extraSections.customCssTip') }}
                </div>
                <div class="mt-3 rounded-xl border border-gray-100 bg-white/70">
                  <div class="px-3 py-2 text-xs font-bold text-gray-700">{{ $t('settings.extraSections.styleVariableTable') }}</div>
                  <div
                    class="grid grid-cols-[minmax(0,1fr)_minmax(0,2fr)_minmax(0,1fr)] gap-y-2 gap-x-3 px-3 pb-3 text-xs"
                  >
                    <div class="text-[10px] font-semibold text-gray-400">{{ $t('settings.extraSections.styleVarName') }}</div>
                    <div class="text-[10px] font-semibold text-gray-400">{{ $t('settings.extraSections.styleVarDesc') }}</div>
                    <div class="text-[10px] font-semibold text-gray-400">{{ $t('settings.extraSections.styleVarValue') }}</div>
                    <template v-for="row in styleVariableRows" :key="row.name">
                      <div class="font-mono text-[11px] text-gray-700">{{ row.name }}</div>
                      <div class="text-gray-500">{{ row.desc }}</div>
                      <div class="font-mono text-[11px] text-gray-600">{{ row.value }}</div>
                    </template>
                  </div>
                  <div
                    class="flex items-center justify-between px-3 py-2 border-t border-gray-100 text-[10px] text-gray-500"
                  >
                    <span>{{ $t('settings.extraSections.styleVarContrast') }}{{ styleVariableStatus.contrast }}</span>
                    <span>{{ $t('settings.extraSections.styleVarVisual') }}{{ styleVariableStatus.visual }}</span>
                  </div>
                </div>
              </div>
            </div>

            <!-- Custom JS Section -->
            <div class="bg-gray-50 border border-gray-100 rounded-xl p-4 mb-6">
              <h4 class="text-base font-bold mb-4 text-gray-900">{{ $t('settings.extraSections.customJs') }}</h4>

              <div
                v-if="!store.appConfig.customJsDisclaimerAgreed"
                class="p-4 bg-white rounded-lg border border-gray-200 shadow-sm"
              >
                <h5 class="font-bold text-gray-700 mb-2 flex items-center gap-2">{{ $t('settings.extraSections.jsDisclaimerTitle') }}</h5>
                <div class="text-sm text-gray-600 mb-3 leading-relaxed">
                  {{ $t('settings.extraSections.jsDisclaimerDesc1') }}
                  <ul class="list-disc list-inside ml-2 mt-1 space-y-1 text-xs">
                    <li>{{ $t('settings.extraSections.jsDisclaimerXss') }}</li>
                    <li>{{ $t('settings.extraSections.jsDisclaimerCrash') }}</li>
                    <li>{{ $t('settings.extraSections.jsDisclaimerLeak') }}</li>
                  </ul>
                </div>
                <p class="text-sm text-gray-600 mb-4 font-bold">
                  {{ $t('settings.extraSections.jsDisclaimerDesc2') }}
                </p>
                <label class="flex items-center gap-2 cursor-pointer select-none">
                  <input
                    type="checkbox"
                    v-model="store.appConfig.customJsDisclaimerAgreed"
                    class="w-4 h-4 text-gray-900 rounded border-gray-300 focus:ring-blue-400 accent-blue-400"
                  />
                  <span class="text-sm font-medium text-gray-700"
                    >{{ $t('settings.extraSections.jsDisclaimerAgree') }}</span
                  >
                </label>
              </div>

              <div v-else>
                <ScriptManager
                  v-if="store.appConfig.customJsList"
                  v-model="store.appConfig.customJsList"
                  type="js"
                  placeholder="// 输入自定义 JS 代码
console.log('Hello from Custom JS!');
document.querySelector('.card-item').addEventListener('click', () => {
  alert('Clicked!');
});"
                  @change="store.updateCustomScripts()"
                />
                <div class="text-xs text-gray-500 mt-2 flex justify-between items-center">
                  <span>{{ $t('settings.extraSections.customJsTip') }}</span>
                  <button
                    @click="store.appConfig.customJsDisclaimerAgreed = false"
                    class="text-xs text-gray-500 hover:text-gray-600 underline"
                  >
                    撤销免责声明并禁用
                  </button>
                </div>
              </div>
            </div>

            <!-- Weather Service Settings -->
            <div class="bg-gray-50 border border-gray-100 rounded-xl p-4 mb-6">
              <div class="flex items-center justify-between mb-4">
                <h4 class="text-base font-bold text-gray-900">{{ $t('settings.extraSections.weatherServiceSettings') }}</h4>
              </div>
              <div class="space-y-3">
                <div>
                  <label class="block text-xs font-bold text-gray-600 mb-2">{{ $t('settings.extraSections.weatherSourceSelect') }}</label>
                  <div class="flex items-center gap-4 mb-3">
                    <label class="flex items-center gap-2 cursor-pointer">
                      <input
                        type="radio"
                        v-model="store.appConfig.weatherSource"
                        value="uapi"
                        class="text-gray-900 accent-blue-400"
                      />
                      <span class="text-sm">{{ $t('settings.extraSections.openMeteo') }}</span>
                    </label>
                    <label class="flex items-center gap-2 cursor-pointer">
                      <input
                        type="radio"
                        v-model="store.appConfig.weatherSource"
                        value="amap"
                        class="text-gray-900 accent-blue-400"
                      />
                      <span class="text-sm">{{ $t('settings.extraSections.amapSource') }}</span>
                    </label>
                    <label class="flex items-center gap-2 cursor-pointer">
                      <input
                        type="radio"
                        v-model="store.appConfig.weatherSource"
                        value="qweather"
                        class="text-gray-900 accent-blue-400"
                      />
                      <span class="text-sm">{{ $t('settings.extraSections.qweatherSource') }}</span>
                    </label>
                  </div>

                  <!-- OpenMeteo Settings -->
                  <div v-if="(!store.appConfig.weatherSource || store.appConfig.weatherSource === 'uapi')" class="animate-fade-in mt-2 mb-4 p-3 bg-blue-50 rounded-lg border border-blue-100">
                    <p class="text-xs text-blue-700 mb-2">
                      {{ $t('settings.extraSections.openMeteoDesc') }}
                    </p>
                    <div class="flex items-center gap-2">
                      <button
                        @click="testWeatherConnection('uapi')"
                        class="px-3 py-1.5 bg-blue-600 hover:bg-blue-700 text-white text-xs rounded transition-colors flex items-center gap-1"
                        :disabled="isTestingWeather"
                      >
                        <span v-if="isTestingWeather" class="animate-spin text-xs">●</span>
                        {{ isTestingWeather ? $t('settings.extraSections.weatherTesting') : $t('settings.extraSections.weatherTestBtn') }}
                      </button>
                      <span
                        v-if="testWeatherResult"
                        class="text-xs"
                        :class="testWeatherResult.success ? 'text-green-600' : 'text-red-500'"
                      >
                        {{ testWeatherResult.message }}
                      </span>
                    </div>
                  </div>
                </div>

                <div v-if="store.appConfig.weatherSource === 'amap'" class="animate-fade-in">
                  <label class="block text-xs font-bold text-gray-600 mb-1">{{ $t('settings.extraSections.amapApiKey') }}</label>
                  <input
                    v-model="store.appConfig.amapKey"
                    class="w-full px-3 py-2 border border-gray-200 rounded-lg text-sm focus:border-gray-900 outline-none"
                    :placeholder="$t('settings.extraSections.placeholderAmapKey')"
                  />
                  <div class="flex items-center justify-between mt-2">
                    <p class="text-[10px] text-gray-500">
                      {{ $t('settings.extraSections.applyForAmapKey') }}
                    </p>
                    <button
                      @click="addAmapWeatherWidget"
                      class="px-3 py-1 text-xs font-medium bg-blue-50 text-blue-600 rounded-lg hover:bg-blue-100 transition-colors flex items-center gap-1"
                    >
                      <span>+</span> {{ $t('settings.extraSections.addWeatherComponentToDesktop') }}
                    </button>
                  </div>
                </div>

                <div
                  v-if="store.appConfig.weatherSource === 'qweather'"
                  class="animate-fade-in space-y-2"
                >
                  <div>
                    <label class="block text-xs font-bold text-gray-600 mb-1">Project ID</label>
                    <input
                      v-model="store.appConfig.qweatherProjectId"
                      class="w-full px-3 py-2 border border-gray-200 rounded-lg text-sm focus:border-gray-900 outline-none"
                      placeholder="请输入 Project ID"
                    />
                  </div>
                  <div>
                    <label class="block text-xs font-bold text-gray-600 mb-1">Key ID</label>
                    <input
                      v-model="store.appConfig.qweatherKeyId"
                      class="w-full px-3 py-2 border border-gray-200 rounded-lg text-sm focus:border-gray-900 outline-none"
                      placeholder="请输入 Key ID"
                    />
                  </div>
                  <div>
                    <label class="block text-xs font-bold text-gray-600 mb-1">Private Key</label>
                    <textarea
                      v-model="store.appConfig.qweatherPrivateKey"
                      class="w-full px-3 py-2 border border-gray-200 rounded-lg text-sm focus:border-gray-900 outline-none min-h-[80px]"
                      placeholder="请输入 Private Key (需包含 -----BEGIN PRIVATE KEY----- 头尾)"
                    ></textarea>
                  </div>
                  <p class="text-[10px] text-gray-500 mt-1">
                    请前往
                    <a
                      href="https://console.qweather.com/"
                      target="_blank"
                      class="text-gray-600 underline hover:text-gray-900"
                      >和风天气控制台</a
                    >
                    获取 JWT 凭证。
                  </p>
                  <div class="flex items-center gap-2 mt-2">
                    <button
                      @click="testWeatherConnection('qweather')"
                      class="px-3 py-1.5 bg-gray-900 hover:bg-gray-800 text-white text-xs rounded transition-colors flex items-center gap-1"
                      :disabled="isTestingWeather"
                    >
                      <span v-if="isTestingWeather" class="animate-spin text-xs">●</span>
                      {{ isTestingWeather ? "测试中..." : "测试连接" }}
                    </button>
                    <span
                      v-if="testWeatherResult"
                      class="text-xs"
                      :class="testWeatherResult.success ? 'text-gray-600' : 'text-gray-500'"
                    >
                      {{ testWeatherResult.message }}
                    </span>
                  </div>
                </div>

                <div>
                  <label class="block text-xs font-bold text-gray-600 mb-1">自定义天气源 URL</label>
                  <input
                    v-model="store.appConfig.weatherApiUrl"
                    class="w-full px-3 py-2 border border-gray-200 rounded-lg text-sm focus:border-gray-900 outline-none"
                    placeholder="默认使用内置源，输入 URL 以自定义"
                  />
                  <p class="text-[10px] text-gray-500 mt-1">
                    若填写，将直接请求该地址获取天气数据。返回格式需包含：
                    <code>{ data: { temp, text, city, humidity, today: { min, max } } }</code>
                  </p>
                </div>
              </div>
            </div>

            <!-- Webhook Settings -->
            <div class="bg-gray-50 border border-gray-100 rounded-xl p-4">
              <div class="flex items-center justify-between mb-4">
                <h4 class="text-base font-bold text-gray-900">{{ $t('settings.extraSections.webhookSettings') }}</h4>
              </div>

              <div class="mb-6">
                <h5 class="font-bold text-gray-900 mb-2">{{ $t('settings.extraSections.webhookAddress') }}</h5>
                <div class="flex items-center gap-2 bg-white/60 p-2 rounded border border-gray-200">
                  <code class="text-xs text-gray-600 flex-1 break-all">{{ getWebhookUrl() }}</code>
                  <button
                    @click="copyWebhookUrl"
                    class="text-xs bg-blue-100 text-blue-600 px-2 py-1 rounded hover:bg-blue-200 font-bold transition-colors"
                  >
                    复制
                  </button>
                </div>
                <p class="text-xs text-gray-500 mt-2">
                  请在 STUN 穿透配置中，将全局 Webhook 的地址设置为上述地址，并使用以下配置：
                </p>

                <div class="mt-3 space-y-3 bg-white/60 p-3 rounded-lg border border-gray-200">
                  <div>
                    <div class="flex items-center gap-2 mb-1">
                      <span class="text-xs font-bold text-gray-700">{{ $t('settings.extraSections.webhookHeader') }}</span>
                    </div>
                    <code
                      class="block text-xs text-gray-600 font-mono bg-gray-50 p-1.5 rounded border border-gray-200"
                      >Content-Type: application/json</code
                    >
                  </div>
                  <div>
                    <div class="flex items-center gap-2 mb-1">
                      <span class="text-xs font-bold text-gray-700">{{ $t('settings.extraSections.webhookBody') }}</span>
                    </div>
                    <pre
                      class="text-xs text-gray-600 font-mono bg-gray-50 p-1.5 rounded border border-gray-200 whitespace-pre"
                    >
{
  "stun": "success",
  "ip": "#{ip}",
  "port": "#{port}"
}</pre
                    >
                  </div>
                </div>
              </div>

              <div class="space-y-3">
                <h5 class="font-bold text-gray-900">{{ $t('settings.extraSections.latestStatus') }}</h5>
                <div
                  v-if="store.luckyStunData && store.luckyStunData.data"
                  class="grid grid-cols-2 gap-3"
                >
                  <div class="bg-white/60 p-3 rounded-lg border border-gray-200">
                    <div class="text-xs text-gray-500 mb-1">{{ $t('settings.extraSections.statusLabel') }}</div>
                    <div
                      class="font-bold"
                      :class="
                        store.luckyStunData.data.stun === 'success'
                          ? 'text-gray-900'
                          : 'text-gray-500'
                      "
                    >
                      {{ store.luckyStunData.data.stun || "未知" }}
                    </div>
                  </div>
                  <div class="bg-white/60 p-3 rounded-lg border border-gray-200">
                    <div class="text-xs text-gray-500 mb-1">{{ $t('settings.extraSections.pubIp') }}</div>
                    <div class="font-bold text-gray-900 font-mono break-all">
                      {{ store.luckyStunData.data.ip || "-" }}
                    </div>
                  </div>
                  <div class="bg-white/60 p-3 rounded-lg border border-gray-200">
                    <div class="text-xs text-gray-500 mb-1">{{ $t('settings.extraSections.port') }}</div>
                    <div class="font-bold text-gray-900">
                      {{ store.luckyStunData.data.port || "-" }}
                    </div>
                  </div>
                  <div class="bg-white/60 p-3 rounded-lg border border-gray-200">
                    <div class="text-xs text-gray-500 mb-1">{{ $t('settings.extraSections.updateTime') }}</div>
                    <div class="text-xs text-gray-900">
                      {{ formatTime(store.luckyStunData.ts) }}
                    </div>
                  </div>
                </div>
                <div
                  v-else
                  class="text-center py-8 text-gray-400 text-sm bg-white rounded-xl border border-dashed border-gray-200"
                >
                  {{ $t('settings.extraSections.noWebhookData') }}
                </div>
              </div>

              <div class="flex justify-end mt-4">
                <button
                  @click="store.fetchLuckyStunData"
                  class="text-sm text-gray-500 hover:text-gray-900 hover:underline flex items-center gap-1 font-bold transition-colors"
                >
                  <span>🔄</span> {{ $t('settings.extraSections.refreshData') }}
                </button>
              </div>
            </div>

          </div>

          <div v-if="activeTab === 'account'" class="min-h-full flex flex-col justify-center">
            <div v-if="!store.isLogged" class="text-center">
              <h4 class="text-xl font-bold mb-6 text-gray-900">{{ $t('settings.extraSections.loginAsAdmin') }}</h4>
              <input
                v-model="passwordInput"
                type="password"
                :placeholder="$t('settings.extraSections.placeholderPasswordLogin')"
                class="w-full max-w-xs px-4 py-3 border border-gray-200 rounded-xl mb-4 mx-auto text-center focus:border-gray-900 outline-none"
                @keyup.enter="handleLogin"
              />
              <button
                @click="handleLogin"
                class="bg-gray-900 text-white px-10 py-3 rounded-xl font-bold hover:bg-gray-800 transition-colors"
              >
                登 录
              </button>
            </div>
            <div v-else class="max-w-sm mx-auto w-full">
              <div class="bg-gray-50 p-5 rounded-xl border border-gray-100 mb-6">
                <h5 class="text-sm font-bold text-gray-900 mb-3">{{ $t('settings.extraSections.backupRestoreSection') }}</h5>
                <div class="grid grid-cols-2 gap-3">
                  <button
                    @click="handleExport"
                    class="col-span-2 bg-white text-gray-700 border border-gray-200 px-4 py-2 rounded-lg text-sm font-bold hover:bg-gray-50 transition-colors"
                  >
                    📤 导出配置
                  </button>
                  <button
                    @click="triggerImport"
                    class="col-span-2 bg-gray-900 text-white px-4 py-2 rounded-lg text-sm font-bold hover:bg-gray-800 transition-colors"
                  >
                    📥 导入配置
                  </button>
                  <button
                    v-if="hasAdminAccess"
                    @click="handleSaveAsDefault"
                    class="bg-gray-800 text-white px-4 py-2 rounded-lg text-sm font-bold hover:bg-gray-900 transition-all"
                  >
                    {{ saveDefaultBtnText }}
                  </button>
                  <button
                    @click="handleReset"
                    class="bg-white text-gray-600 border border-gray-200 px-4 py-2 rounded-lg text-sm font-bold hover:bg-red-50 hover:text-red-600 hover:border-red-200 transition-colors"
                  >
                    🧹 恢复初始化
                  </button>
                  <input
                    ref="fileInput"
                    type="file"
                    accept=".navbak,.json"
                    class="hidden"
                    @change="handleFileChange"
                  />
                </div>
              </div>
              <div
                v-if="hasAdminAccess"
                class="bg-gray-50 p-5 rounded-xl border border-gray-200 mb-6"
              >
                <h5 class="text-sm font-bold text-gray-900 mb-3">{{ $t('settings.extraSections.systemMode') }}</h5>
                <div class="flex items-center justify-between">
                  <span class="text-sm text-gray-700"
                    >当前模式：{{
                      store.systemConfig.authMode === "single" ? $t('settings.extraSections.singleUserMode') : $t('settings.extraSections.multiUserMode')
                    }}</span
                  >
                  <button
                    v-if="hasAdminAccess"
                    @click="toggleAuthMode"
                    class="px-4 py-2 rounded-lg text-sm font-bold text-white transition-all bg-gray-900 hover:bg-gray-800"
                  >
                    {{ $t('settings.extraSections.switchToMode', { mode: store.systemConfig.authMode === 'single' ? $t('settings.extraSections.multiUserMode') : $t('settings.extraSections.singleUserMode') }) }}
                  </button>
                  <span
                    v-else
                    class="px-4 py-2 rounded-lg text-sm font-bold text-gray-500 bg-gray-100"
                  >
                    仅管理员可切换
                  </span>
                </div>
                <p class="text-xs text-gray-500 mt-2">
                  {{
                    store.systemConfig.authMode === "single"
                      ? $t('settings.extraSections.singleModeHint')
                      : $t('settings.extraSections.multiModeHint')
                  }}
                </p>
                <p class="text-xs text-gray-500 mt-1">
                  {{ $t('settings.extraSections.defaultPasswords') }}
                </p>
              </div>
              <div class="bg-gray-50 p-5 rounded-xl border border-gray-200 mb-6">
                <h5 class="text-sm font-bold text-gray-900 mb-3">{{ $t('settings.extraSections.configVersionsSection') }}</h5>
                <div class="flex gap-2 items-center mb-2">
                  <input
                    v-model="versionLabel"
                    :placeholder="$t('settings.extraSections.placeholderVersionLabel')"
                    class="flex-1 px-3 py-2 rounded-lg border border-gray-300 text-sm focus:border-gray-900 outline-none"
                  />
                  <button
                    @click="saveVersion"
                    class="px-4 py-2 rounded-lg text-sm font-bold text-white transition-all bg-gray-900 hover:bg-gray-800"
                  >
                    保存为版本
                  </button>
                </div>
                <div class="text-[10px] text-gray-500 mb-2">
                  {{
                    store.systemConfig.authMode === "single"
                      ? $t('settings.extraSections.saveLocation')
                      : $t('settings.extraSections.saveLocationUser')
                  }}
                </div>
                <div class="max-h-40 overflow-y-auto space-y-1">
                  <div v-if="loadingVersions" class="text-xs text-gray-500">{{ $t('settings.extraSections.loadingVersions') }}</div>
                  <div v-else-if="versions.length === 0" class="text-xs text-gray-400 text-center py-4">
                    {{ $t('settings.extraSections.noVersionsSaved') }}
                  </div>
                  <div
                    v-for="v in versions"
                    :key="v.id"
                    class="flex items-center justify-between bg-white px-3 py-2 rounded-lg border border-gray-200"
                  >
                    <div class="flex-1">
                      <div class="text-sm font-medium text-gray-900 truncate">
                        {{ v.label || $t('settings.extraSections.unnamedVersion') }}
                      </div>
                      <div class="text-[10px] text-gray-500">
                        {{ new Date(v.createdAt).toLocaleString() }} ·
                        {{ Math.round(v.size / 1024) }}KB
                      </div>
                    </div>
                    <div class="flex gap-2">
                      <button
                        @click="restoreVersion(v.id)"
                        class="text-xs px-2 py-1 rounded bg-gray-200 text-gray-700 hover:bg-gray-300 transition-colors"
                      >
                        恢复
                      </button>
                      <button
                        @click="deleteVersion(v.id)"
                        class="text-xs px-2 py-1 rounded bg-gray-200 text-gray-700 hover:bg-gray-300 transition-colors"
                      >
                        删除
                      </button>
                    </div>
                  </div>
                </div>
              </div>
              <div class="bg-gray-50 p-5 rounded-xl border border-gray-200 mb-6">
                <h5 class="text-sm font-medium text-gray-700 mb-1">{{ $t('settings.extraSections.changePasswordSection') }}</h5>
                <p class="text-xs text-gray-500 mb-2">{{ $t('settings.extraSections.modifyPasswordHint') }}</p>
                <div class="flex gap-2">
                  <div class="relative flex-1">
                    <input
                      v-model="newPasswordInput"
                      :type="showPassword ? 'text' : 'password'"
                      :placeholder="$t('settings.extraSections.placeholderNewPassword')"
                      class="w-full px-3 py-2 rounded-lg border border-gray-300 pr-10 focus:border-gray-900 outline-none"
                    />
                    <button
                      @click="showPassword = !showPassword"
                      class="absolute right-2 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-600 focus:outline-none"
                      type="button"
                      tabindex="-1"
                      :title="showPassword ? $t('settings.extraSections.hidePassword') : $t('settings.extraSections.showPassword')"
                    >
                      <svg
                        v-if="showPassword"
                        xmlns="http://www.w3.org/2000/svg"
                        fill="none"
                        viewBox="0 0 24 24"
                        stroke-width="1.5"
                        stroke="currentColor"
                        class="w-5 h-5"
                      >
                        <path
                          stroke-linecap="round"
                          stroke-linejoin="round"
                          d="M3.98 8.223A10.477 10.477 0 001.934 12C3.226 16.338 7.244 19.5 12 19.5c.993 0 1.953-.138 2.863-.395M6.228 6.228A10.45 10.45 0 0112 4.5c4.756 0 8.773 3.162 10.065 7.498a10.523 10.523 0 01-4.293 5.774M6.228 6.228L3 3m3.228 3.228l3.65 3.65m7.894 7.894L21 21m-3.228-3.228l-3.65-3.65m0 0a3 3 0 10-4.243-4.243m4.242 4.242L9.88 9.88"
                        />
                      </svg>
                      <svg
                        v-else
                        xmlns="http://www.w3.org/2000/svg"
                        fill="none"
                        viewBox="0 0 24 24"
                        stroke-width="1.5"
                        stroke="currentColor"
                        class="w-5 h-5"
                      >
                        <path
                          stroke-linecap="round"
                          stroke-linejoin="round"
                          d="M2.036 12.322a1.012 1.012 0 010-.639C3.423 7.51 7.36 4.5 12 4.5c4.638 0 8.573 3.007 9.963 7.178.07.207.07.431 0 .639C20.577 16.49 16.64 19.5 12 19.5c-4.638 0-8.573-3.007-9.963-7.178z"
                        />
                        <path
                          stroke-linecap="round"
                          stroke-linejoin="round"
                          d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"
                        />
                      </svg>
                    </button>
                  </div>
                  <button
                    @click="handleChangePassword"
                    class="bg-gray-900 text-white px-4 py-2 rounded-lg text-sm hover:bg-gray-800 transition-colors"
                  >
                    修改
                  </button>
                </div>
              </div>
              <!-- Admin User Management UI -->
              <div
                v-if="canManageUsers"
                class="bg-gray-50 p-5 rounded-xl border border-gray-200 mb-6"
              >
                <h5 class="text-sm font-bold text-gray-900 mb-3">👥 用户管理 (Admin)</h5>

                <!-- Add User -->
                <div class="flex flex-col gap-2 mb-4">
                  <div class="flex gap-2">
                    <input
                      v-model="newUser"
                      placeholder="用户名"
                      class="flex-1 px-3 py-2 rounded-lg border border-gray-300 text-sm focus:border-gray-900 outline-none"
                    />
                    <input
                      v-model="newPwd"
                      type="password"
                      placeholder="密码"
                      class="flex-1 px-3 py-2 rounded-lg border border-gray-300 text-sm focus:border-gray-900 outline-none"
                    />
                  </div>
                  <button
                    @click="handleAddUser"
                    class="bg-gray-900 text-white px-4 py-2 rounded-lg text-sm font-bold hover:bg-gray-800 transition-colors"
                  >
                    添加用户
                  </button>
                </div>

                <!-- User List -->
                <div class="space-y-2 max-h-40 overflow-y-auto">
                  <div
                    v-for="u in userList"
                    :key="u"
                    class="flex justify-between items-center bg-white px-3 py-2 rounded-lg border border-gray-200"
                  >
                    <span class="text-sm text-gray-700 font-medium">
                      {{ u }}
                      <span v-if="u === 'admin'" class="text-xs text-gray-500">{{ $t('settings.extraSections.admin') }}</span>
                    </span>
                    <button
                      v-if="u !== 'admin'"
                      @click="handleDeleteUser(u)"
                      class="text-gray-400 hover:text-gray-600 text-xs font-bold px-2"
                    >
                      删除
                    </button>
                  </div>
                </div>

                <!-- License Management -->
                <div class="mt-4 pt-4 border-t border-gray-200">
                  <h6 class="text-xs font-bold text-gray-900 mb-2">🔑 授权密钥 (License Key)</h6>
                  <div class="flex gap-2">
                    <input
                      v-model="licenseKey"
                      :placeholder="$t('settings.extraSections.placeholderLicenseKey')"
                      class="flex-1 px-3 py-2 rounded-lg border border-gray-300 text-sm focus:border-gray-900 outline-none"
                    />
                    <button
                      @click="handleUploadLicense"
                      class="bg-gray-900 text-white px-3 py-2 rounded-lg text-sm font-bold hover:bg-gray-800 whitespace-nowrap transition-colors"
                    >
                      导入
                    </button>
                  </div>
                  <p class="text-[10px] text-gray-500 mt-1">
                    导入有效密钥可解除5个用户的注册限制。
                  </p>
                </div>
              </div>

              <!-- 云栖阁：MCP 服务端配置（给外部 AI 助手用） -->
              <div class="bg-gray-50 p-5 rounded-xl border border-gray-200 mb-6">
                <h5 class="text-sm font-bold text-gray-900 mb-2">🔌 MCP 服务端 <span class="text-xs font-normal text-gray-500">（给 Claude / 其他 AI 助手连接用）</span></h5>
                <p class="text-xs text-gray-500 mb-3">外部 AI 助手通过 MCP 协议可搜索、增删你的书签。端点与钥匙如下：</p>
                <div class="space-y-2 text-xs">
                  <div>
                    <label class="text-gray-500 block mb-1">MCP 端点（POST，JSON-RPC 2.0）</label>
                    <code class="block px-3 py-2 rounded-lg bg-white border border-gray-200 font-mono break-all">{{ mcpEndpoint }}</code>
                  </div>
                  <div>
                    <label class="text-gray-500 block mb-1">钥匙（Token，请求头 Authorization: Bearer 或 token 头）</label>
                    <div class="flex items-center gap-2">
                      <code class="flex-1 min-w-0 px-3 py-2 rounded-lg bg-white border border-gray-200 font-mono truncate">
                        {{ tokenLoading ? "读取中…" : showYunqiToken ? yunqiToken : maskedToken }}
                      </code>
                      <button
                        @click="showYunqiToken = !showYunqiToken"
                        class="px-3 py-2 rounded-lg text-xs font-bold bg-white border border-gray-200 text-gray-700 hover:bg-gray-100 whitespace-nowrap"
                      >
                        {{ showYunqiToken ? $t('common.yunqi.settings.tokenHide') : $t('common.yunqi.settings.tokenShow') }}
                      </button>
                      <button
                        @click="copyYunqiToken"
                        class="px-3 py-2 rounded-lg text-xs font-bold bg-white border border-gray-200 text-gray-700 hover:bg-gray-100 whitespace-nowrap"
                      >
                        {{ $t('common.yunqi.settings.tokenCopy') }}
                      </button>
                    </div>
                  </div>
                  <div>
                    <label class="text-gray-500 block mb-1">可用工具（{{ mcpToolCount }} 个）</label>
                    <p class="text-gray-600 leading-relaxed">搜索书签 · 分类列表 · 分类书签 · 查书签详情 · 新增/修改/删除书签 · 签到管理 · AI 问答 · 系统状态</p>
                  </div>
                  <div>
                    <label class="text-gray-500 block mb-1">🤖 AI 智能安装提示词（复制发给 AI 助手，它会自动帮你配好）</label>
                    <div class="relative">
                      <pre class="px-3 py-2 rounded-lg bg-white border border-gray-200 font-mono text-xs whitespace-pre-wrap break-all max-h-40 overflow-y-auto">{{ mcpInstallPrompt }}</pre>
                      <button
                        @click="copyMcpPrompt"
                        class="mt-2 px-3 py-1.5 rounded-lg text-xs font-bold bg-gray-900 text-white hover:bg-gray-700"
                      >📋 复制提示词</button>
                    </div>
                  </div>
                </div>
                <button
                  @click="regenerateYunqiToken"
                  :disabled="tokenLoading"
                  class="mt-3 w-full bg-gray-900 text-white px-4 py-2 rounded-lg text-sm font-bold hover:bg-gray-800 transition-colors disabled:opacity-50"
                >
                  🔑 {{ $t('common.yunqi.settings.tokenRegen') }}
                </button>
              </div>

              <!-- 云栖阁：n8n 工作流接口 -->
              <div class="bg-gray-50 p-5 rounded-xl border border-gray-200 mb-6">
                <h5 class="text-sm font-bold text-gray-900 mb-2">🔄 n8n 工作流接口 <span class="text-xs font-normal text-gray-500">（IT&AI 日报自动推送用）</span></h5>
                <p class="text-xs text-gray-500 mb-3">n8n 上的「Workflow-02 IT&AI日报」每天 19 点生成日报后，推送到下面接口，云栖阁首页的「IT&AI 日报」小组件会自动更新。钥匙与上方 MCP 共用同一把。</p>
                <div class="space-y-1 text-xs font-mono">
                  <div class="px-3 py-1.5 rounded-lg bg-white border border-gray-200 break-all">POST {{ openapiBase }}/itdaily/push <span class="text-gray-400">— 推送日报（Body 为日报 JSON，Header 带 Authorization）</span></div>
                </div>
                <p class="text-xs text-green-600 mt-2">✅ OECT 的 n8n 工作流已配好推送节点，每天自动同步</p>
              </div>

              <!-- 云栖阁：AI 聊天模型设置 -->
              <div class="bg-gray-50 p-5 rounded-xl border border-gray-200 mb-6">
                <h5 class="text-sm font-bold text-gray-900 mb-3">{{ $t('common.yunqi.settings.aiSection') }}</h5>
                <div class="space-y-2">
                  <div>
                    <label class="text-xs text-gray-500 block mb-1">AI 平台</label>
                    <select
                      v-model="aiProvider"
                      @change="onAiProviderChange"
                      class="w-full px-3 py-2 rounded-lg border border-gray-300 text-sm focus:border-gray-900 outline-none bg-white"
                    >
                      <option v-for="p in aiPresets" :key="p.name" :value="p.name">{{ p.name }}</option>
                    </select>
                    <div v-if="aiPresets.find((x) => x.name === aiProvider)?.key_url" class="mt-1">
                      <a
                        :href="aiPresets.find((x) => x.name === aiProvider)?.key_url"
                        target="_blank"
                        rel="noopener"
                        class="text-xs text-blue-500 hover:underline"
                      >👉 去申请 API Key</a>
                    </div>
                  </div>
                  <div>
                    <label class="text-xs text-gray-500 block mb-1">{{ $t('common.yunqi.settings.aiBaseUrl') }}</label>
                    <input
                      v-model="aiBaseUrl"
                      placeholder="https://api.deepseek.com/v1"
                      class="w-full px-3 py-2 rounded-lg border border-gray-300 text-sm focus:border-gray-900 outline-none"
                    />
                  </div>
                  <div>
                    <label class="text-xs text-gray-500 block mb-1">
                      {{ $t('common.yunqi.settings.aiApiKey') }}
                      <span v-if="aiHasKey" class="text-green-600">（已配置好，不改就别填）</span>
                    </label>
                    <input
                      v-model="aiApiKey"
                      type="password"
                      placeholder="sk-…（不改动请留空）"
                      autocomplete="new-password"
                      class="w-full px-3 py-2 rounded-lg border border-gray-300 text-sm focus:border-gray-900 outline-none"
                    />
                  </div>
                  <div>
                    <label class="text-xs text-gray-500 block mb-1">{{ $t('common.yunqi.settings.aiModel') }}</label>
                    <input
                      v-model="aiModel"
                      placeholder="deepseek-chat"
                      class="w-full px-3 py-2 rounded-lg border border-gray-300 text-sm focus:border-gray-900 outline-none"
                    />
                  </div>
                  <div class="pt-1">
                    <label class="flex items-center gap-2 text-xs text-gray-700 cursor-pointer">
                      <input type="checkbox" v-model="aiProxyEnabled" class="w-4 h-4" />
                      国际模型走代理（推荐：OpenRouter / Groq / Google 等国外平台开启）
                    </label>
                    <input
                      v-if="aiProxyEnabled"
                      v-model="aiProxyUrl"
                      placeholder="http://192.168.57.228:7890"
                      class="mt-1 w-full px-3 py-2 rounded-lg border border-gray-300 text-sm focus:border-gray-900 outline-none"
                    />
                    <p v-if="aiProxyEnabled" class="text-xs text-gray-400 mt-1">默认用 J4125 上 sing-box 的代理（7890 端口），不用改</p>
                  </div>
                  <button
                    @click="saveAiConfig"
                    :disabled="aiSaving"
                    class="w-full bg-gray-900 text-white px-4 py-2 rounded-lg text-sm font-bold hover:bg-gray-800 transition-colors disabled:opacity-50"
                  >
                    {{ aiSaving ? "保存中…" : $t('common.save') }}
                  </button>
                  <p class="text-xs text-gray-400">💡 AI 助手现在可以帮你：找网址、收藏新网址、删除网址、修改网址信息、查看分类</p>
                </div>
              </div>

              <!-- 云栖阁：登录二次验证（OTP） -->
              <div class="bg-gray-50 p-5 rounded-xl border border-gray-200 mb-6">
                <h5 class="text-sm font-bold text-gray-900 mb-2">🛡️ {{ $t('common.yunqi.settings.otpSection') }}</h5>
                <div class="flex items-center justify-between mb-2">
                  <span
                    class="text-xs font-bold px-2 py-1 rounded-full"
                    :class="otpBound ? 'bg-green-100 text-green-700' : 'bg-gray-200 text-gray-600'"
                  >
                    {{ otpBound ? $t('common.yunqi.settings.otpEnabled') : $t('common.yunqi.settings.otpNotEnabled') }}
                  </span>
                </div>

                <!-- 未绑定：开始绑定 -->
                <div v-if="!otpBound && !otpSettingUp">
                  <button
                    @click="startOtpSetup"
                    :disabled="otpBusy"
                    class="w-full bg-gray-900 text-white px-4 py-2 rounded-lg text-sm font-bold hover:bg-gray-800 transition-colors disabled:opacity-50"
                  >
                    {{ otpBusy ? "准备中…" : $t('common.yunqi.settings.otpStart') }}
                  </button>
                </div>

                <!-- 绑定中：二维码 + 密钥 + 验证码 -->
                <div v-if="!otpBound && otpSettingUp" class="space-y-3">
                  <p class="text-xs text-gray-500 leading-relaxed">{{ $t('common.yunqi.settings.otpScanHint') }}</p>
                  <div class="flex justify-center bg-white p-3 rounded-xl border border-gray-200">
                    <QrcodeVue :value="otpSetup.otpauth_url" :size="180" level="M" />
                  </div>
                  <div>
                    <label class="text-xs text-gray-500 block mb-1">{{ $t('common.yunqi.settings.otpSecret') }}</label>
                    <code class="block px-3 py-2 rounded-lg bg-white border border-gray-200 text-sm font-mono break-all select-all">{{ otpSetup.secret }}</code>
                  </div>
                  <input
                    v-model="otpCode"
                    type="text"
                    inputmode="numeric"
                    maxlength="6"
                    :placeholder="$t('common.yunqi.settings.otpCodePh')"
                    class="w-full px-4 py-2 rounded-lg border border-gray-300 text-center text-xl tracking-[0.4em] focus:border-gray-900 outline-none"
                    @keyup.enter="confirmOtpEnable"
                  />
                  <div class="flex gap-2">
                    <button
                      @click="otpSettingUp = false"
                      class="flex-1 bg-white border border-gray-200 text-gray-700 px-4 py-2 rounded-lg text-sm font-bold hover:bg-gray-100"
                    >
                      {{ $t('common.cancel') }}
                    </button>
                    <button
                      @click="confirmOtpEnable"
                      :disabled="otpBusy"
                      class="flex-1 bg-gray-900 text-white px-4 py-2 rounded-lg text-sm font-bold hover:bg-gray-800 disabled:opacity-50"
                    >
                      {{ otpBusy ? "验证中…" : $t('common.yunqi.settings.otpEnable') }}
                    </button>
                  </div>
                </div>

                <!-- 已绑定：关闭 -->
                <div v-if="otpBound" class="space-y-2">
                  <input
                    v-model="otpCode"
                    type="text"
                    inputmode="numeric"
                    maxlength="6"
                    :placeholder="$t('common.yunqi.settings.otpCodePh')"
                    class="w-full px-4 py-2 rounded-lg border border-gray-300 text-center text-xl tracking-[0.4em] focus:border-gray-900 outline-none"
                    @keyup.enter="disableOtp"
                  />
                  <button
                    @click="disableOtp"
                    :disabled="otpBusy"
                    class="w-full bg-white border border-red-200 text-red-600 px-4 py-2 rounded-lg text-sm font-bold hover:bg-red-50 disabled:opacity-50"
                  >
                    {{ otpBusy ? "处理中…" : $t('common.yunqi.settings.otpDisable') }}
                  </button>
                </div>
              </div>

              <!-- 云栖阁：可信设备 -->
              <div class="bg-gray-50 p-5 rounded-xl border border-gray-200 mb-6">
                <h5 class="text-sm font-bold text-gray-900 mb-2">📱 {{ $t('common.yunqi.settings.devicesSection') }}</h5>
                <p class="text-xs text-gray-500 mb-3">{{ $t('common.yunqi.settings.devicesDesc') }}</p>
                <div v-if="devicesLoading" class="text-xs text-gray-500 text-center py-3">读取中…</div>
                <div v-else-if="trustedDevices.length === 0" class="text-xs text-gray-400 text-center py-3">
                  {{ $t('common.yunqi.settings.devicesEmpty') }}
                </div>
                <div v-else class="space-y-2 max-h-48 overflow-y-auto">
                  <div
                    v-for="d in trustedDevices"
                    :key="d.id"
                    class="flex items-center justify-between bg-white px-3 py-2 rounded-lg border border-gray-200"
                  >
                    <div class="min-w-0 flex-1 mr-2">
                      <div class="text-sm font-medium text-gray-800 truncate">{{ d.name || d.id }}</div>
                      <div class="text-[10px] text-gray-500">
                        {{ $t('common.yunqi.settings.deviceLastUsed') }}：{{ formatDeviceTime(d.last_used_at) }}
                        · {{ $t('common.yunqi.settings.deviceExpires') }}：{{ formatDeviceTime(d.expires_at) }}
                      </div>
                    </div>
                    <button
                      @click="revokeDevice(d)"
                      class="text-xs px-3 py-1.5 rounded-lg bg-red-50 text-red-600 border border-red-200 hover:bg-red-100 font-bold whitespace-nowrap"
                    >
                      {{ $t('common.yunqi.settings.deviceRevoke') }}
                    </button>
                  </div>
                </div>
              </div>

              <button
                @click="store.logout"
                class="w-full text-gray-700 py-3 rounded-xl font-bold border border-gray-200 transition-colors glass-chip selectable-outline"
              >
                退出登录
              </button>
            </div>
          </div>
          <div v-if="activeTab === 'about'" class="min-h-full flex flex-col p-8 -mt-4">
            <div class="bg-white/60 border border-gray-100 rounded-xl p-4">
              <h5 class="text-sm font-bold text-gray-900 mb-2">云栖阁 YunQi Pavilion</h5>
              <div class="text-xs text-gray-600 leading-relaxed">
                基于 FlatNas 二次开发的导航页，新增 AI 聊天助手、MCP 服务、网站签到提醒、OTP 双重验证、30 天可信设备免验证等功能。
              </div>
            </div>

            <div class="mt-4 bg-white/60 border border-gray-100 rounded-xl p-4">
              <div class="text-xs text-gray-500">
                项目地址：
                <a
                  href="https://github.com/ayongsheng777-rgb/YunQi-Pavilion"
                  target="_blank"
                  class="text-gray-600 underline hover:text-gray-900"
                >
                  https://github.com/ayongsheng777-rgb/YunQi-Pavilion
                </a>
              </div>
              <div class="mt-3 text-xs text-gray-500 leading-relaxed">
                感谢原作者 Garry-QD 的 FlatNas 开源项目：
                <a
                  href="https://github.com/Garry-QD/FlatNas"
                  target="_blank"
                  class="text-gray-600 underline hover:text-gray-900"
                >
                  https://github.com/Garry-QD/FlatNas
                </a>
              </div>
            </div>

            <div class="mt-auto pt-4">
              <div class="bg-white/60 border border-gray-100 rounded-xl p-4">
                <div class="flex items-center justify-end gap-4">
                  <span class="text-lg text-gray-400 font-mono">v{{ store.currentVersion }}</span>
                  <a
                    href="https://github.com/ayongsheng777-rgb/YunQi-Pavilion"
                    target="_blank"
                    class="text-gray-700 hover:text-gray-900 hover:opacity-80 transition-opacity"
                    title="GitHub"
                  >
                    <svg
                      xmlns="http://www.w3.org/2000/svg"
                      viewBox="0 0 24 24"
                      fill="currentColor"
                      class="w-6 h-6"
                    >
                      <path
                        d="M12 .296c-6.63 0-12 5.373-12 12 0 5.303 3.438 9.8 8.205 11.385.6.113.82-.258.82-.577 0-.285-.01-1.04-.015-2.04-3.338.724-4.042-1.61-4.042-1.61-.546-1.385-1.332-1.754-1.332-1.754-1.09-.744.084-.729.084-.729 1.205.084 1.84 1.236 1.84 1.236 1.07 1.835 2.807 1.305 3.492.998.108-.776.418-1.305.762-1.604-2.665-.305-5.467-1.335-5.467-5.932 0-1.31.465-2.38 1.235-3.22-.135-.304-.54-1.523.105-3.176 0 0 1.005-.322 3.3 1.23a11.52 11.52 0 013.003-.405c1.02.006 2.045.138 3.003.405 2.28-1.552 3.285-1.23 3.285-1.23.645 1.653.24 2.872.12 3.176.765.84 1.23 1.91 1.23 3.22 0 4.61-2.805 5.625-5.475 5.922.42.36.81 1.096.81 2.22 0 1.606-.015 2.896-.015 3.286 0 .315.21.69.825.57C20.565 22.092 24 17.592 24 12.296c0-6.627-5.373-12-12-12"
                      />
                    </svg>
                  </a>
                </div>
              </div>
            </div>
          </div>

          <div v-if="activeTab === 'language'" class="space-y-4">
            <div class="bg-white/60 border border-gray-100 rounded-xl p-4">
              <h4 class="text-base font-bold mb-4 text-gray-900">{{ $t('settings.language.title') }}</h4>
              <div class="space-y-3">
                <div
                  v-for="locale in i18nStore.availableLocales"
                  :key="locale"
                  @click="i18nStore.setLocale(locale, true)"
                  :class="[
                    'flex items-center justify-between p-3 rounded-lg cursor-pointer transition-all border',
                    i18nStore.currentLocale === locale
                      ? 'bg-gray-900 text-white border-gray-900'
                      : 'bg-white border-gray-200 hover:border-gray-400'
                  ]"
                >
                  <div class="flex items-center gap-3">
                    <span class="text-lg">{{ LOCALE_FLAGS[locale] }}</span>
                    <span class="font-medium">{{ LOCALE_LABELS[locale] }}</span>
                  </div>
                  <svg
                    v-if="i18nStore.currentLocale === locale"
                    xmlns="http://www.w3.org/2000/svg"
                    viewBox="0 0 24 24"
                    fill="currentColor"
                    class="w-5 h-5"
                  >
                    <path
                      fill-rule="evenodd"
                      d="M2.25 12c0-5.385 4.365-9.75 9.75-9.75s9.75 4.365 9.75 9.75-4.365 9.75-9.75 9.75S2.25 17.385 2.25 12zm13.36-1.814a.75.75 0 10-1.22-.872l-3.236 4.53L9.53 12.22a.75.75 0 00-1.06 1.06l2.25 2.25a.75.75 0 001.14-.094l3.75-5.25z"
                      clip-rule="evenodd"
                    />
                  </svg>
                </div>
              </div>
              <p class="mt-4 text-xs text-gray-500">
                 {{ $t('settings.language.tip') }}
               </p>
            </div>
          </div>
        </div>
      </div>
    </div>
  </OverlayMotion>
  <PasswordConfirmModal
    v-model:show="showPasswordConfirm"
    :title="confirmTitle"
    :on-success="onAuthSuccess"
  />

  <!-- Multi-User Warning Modal -->
  <OverlayMotion
    :show="showMultiUserWarning"
    :z-index="80"
    overlay-class="bg-black/40 backdrop-blur-sm p-4 pt-[calc(1rem+env(safe-area-inset-top))] pb-[calc(1rem+env(safe-area-inset-bottom))]"
    panel-class="max-w-sm"
  >
    <div class="bg-white rounded-xl shadow-xl p-6 w-full border border-gray-100">
      <div class="flex items-center gap-3 mb-4">
        <div class="w-10 h-10 rounded-full bg-gray-100 flex items-center justify-center text-2xl">
          !
        </div>
        <h3 class="text-base font-bold text-gray-900">{{ $t('settings.extraSections.switchModeWarning') }}</h3>
      </div>

      <p class="text-sm text-gray-600 mb-6 leading-relaxed">
        请先导出配置！<br />
        切换到多用户模式会导致当前单用户配置丢失（数据隔离），是否确认继续？
      </p>

      <div class="flex gap-3">
        <button
          @click="showMultiUserWarning = false"
          class="flex-1 px-4 py-2.5 min-h-[44px] bg-gray-100 text-gray-600 rounded-lg text-sm font-bold hover:bg-gray-200 transition-colors"
        >
          取消
        </button>
        <button
          @click="
            showMultiUserWarning = false;
            requestAuth(() => performAuthModeSwitch('multi'), t('settings.messages.confirmSwitchAuth'));
          "
          class="flex-1 px-4 py-2.5 min-h-[44px] bg-gray-900 text-white rounded-lg text-sm font-bold hover:bg-gray-800 transition-colors shadow-md"
        >
          确认切换
        </button>
      </div>
    </div>
  </OverlayMotion>

  <!-- Delete Confirmation Modal -->
  <OverlayMotion
    :show="showDeleteWidgetConfirm"
    :z-index="70"
    overlay-class="bg-black/20 backdrop-blur-sm p-4 pt-[calc(1rem+env(safe-area-inset-top))] pb-[calc(1rem+env(safe-area-inset-bottom))]"
    panel-class="max-w-sm"
  >
    <div class="bg-white rounded-xl shadow-xl p-6 w-full border border-gray-100">
      <h3 class="text-base font-bold text-gray-900 mb-2">{{ $t('settings.extraSections.confirmDeleteTitle') }}</h3>
      <p class="text-sm text-gray-500 mb-6">{{ $t('settings.extraSections.confirmDeleteWindowDesc') }}</p>
      <div class="flex gap-3">
        <button
          @click="showDeleteWidgetConfirm = false"
          class="flex-1 px-4 py-2 min-h-[44px] bg-gray-100 text-gray-600 rounded-lg text-sm font-bold hover:bg-gray-200 transition-colors"
        >
          取消
        </button>
        <button
          @click="confirmRemoveWidget"
          class="flex-1 px-4 py-2 min-h-[44px] bg-gray-900 text-white rounded-lg text-sm font-bold hover:bg-gray-800 transition-colors"
        >
          删除
        </button>
      </div>
    </div>
  </OverlayMotion>

  <OverlayMotion
    :show="showRenameModal"
    :z-index="90"
    overlay-class="bg-black/40 backdrop-blur-sm p-4 pt-[calc(1rem+env(safe-area-inset-top))] pb-[calc(1rem+env(safe-area-inset-bottom))]"
    panel-class="max-w-sm"
  >
    <div class="bg-white rounded-xl shadow-xl w-full p-4 border border-gray-100">
      <div class="text-base font-bold text-gray-900">修改昵称</div>
      <div class="text-xs text-gray-500 mt-1">将同步更新到道理鱼音乐账户资料</div>

      <input
        v-model="newDisplayName"
        type="text"
        class="mt-3 w-full px-3 py-2 border border-gray-200 rounded-lg text-sm focus:border-gray-900 outline-none"
        placeholder="请输入新的昵称"
        @keyup.enter="updateDisplayName"
      />

      <div class="mt-4 flex justify-end gap-2">
        <button
          @click="showRenameModal = false"
          class="px-4 py-2 min-h-[44px] text-gray-600 hover:bg-gray-100 rounded-lg text-sm transition-colors"
        >
          取消
        </button>
        <button
          @click="updateDisplayName"
          :disabled="isUpdatingProfile"
          class="px-4 py-2 min-h-[44px] bg-gray-900 text-white rounded-lg text-sm font-bold hover:bg-gray-800 disabled:opacity-50 transition-colors"
        >
          {{ isUpdatingProfile ? "保存中..." : "保存" }}
        </button>
      </div>
    </div>
  </OverlayMotion>

</template>

<style scoped>
.night-settings {
  color: #f8fafc;
}
.night-settings :deep(.bg-white\/90),
.night-settings :deep(.bg-white\/80),
.night-settings :deep(.bg-white\/70),
.night-settings :deep(.bg-white\/60),
.night-settings :deep(.bg-white),
.night-settings :deep(.bg-gray-50),
.night-settings :deep(.bg-gray-100),
.night-settings :deep(.bg-white\/90):hover,
.night-settings :deep(.bg-white\/80):hover,
.night-settings :deep(.bg-white\/70):hover,
.night-settings :deep(.bg-white\/60):hover,
.night-settings :deep(.bg-white):hover,
.night-settings :deep(.bg-gray-50):hover,
.night-settings :deep(.bg-gray-100):hover {
  background-color: rgba(15, 23, 42, 0.55) !important;
  backdrop-filter: blur(12px);
}
/* 夜间模式：侧栏等使用 hover:bg-gray-50 的按钮悬停时用深色背景，避免与浅色文字同色 */
.night-settings :deep(.hover\:bg-gray-50):hover,
.night-settings :deep(.hover\:bg-gray-100):hover {
  background-color: rgba(15, 23, 42, 0.55) !important;
  backdrop-filter: blur(8px);
}
.night-settings :deep(.text-gray-900),
.night-settings :deep(.text-gray-800),
.night-settings :deep(.text-gray-700),
.night-settings :deep(.text-gray-600),
.night-settings :deep(.text-gray-500),
.night-settings :deep(.text-gray-400) {
  color: #f8fafc !important;
  text-shadow: 0 0 2px rgba(255, 255, 255, 0.6);
}
.night-settings :deep(.border-gray-100),
.night-settings :deep(.border-gray-200),
.night-settings :deep(.border-gray-300),
.night-settings :deep(.border-gray-400) {
  border-color: rgba(255, 255, 255, 0.12) !important;
}
.night-settings :deep(input::placeholder),
.night-settings :deep(textarea::placeholder) {
  color: rgba(248, 250, 252, 0.6);
}
.glass-panel,
.glass-card,
.glass-chip {
  background-color: rgba(255, 255, 255, 0.1);
  backdrop-filter: blur(8px);
  -webkit-backdrop-filter: blur(8px);
  border: 1px solid rgba(255, 255, 255, 0.2);
}
.glass-chip:hover {
  background-color: rgba(255, 255, 255, 0.12);
}
.night-settings :deep(.glass-panel),
.night-settings :deep(.glass-card),
.night-settings :deep(.glass-chip) {
  color: #f8fafc;
  text-shadow: 0 1px 2px rgba(0, 0, 0, 0.55);
}
.selected-outline {
  border: 2px solid rgba(255, 255, 255, 0.95);
  box-shadow: 0 0 0 1px rgba(0, 0, 0, 0.12);
  font-weight: 700;
}
.selectable-outline:focus-visible,
.selectable-outline:active {
  box-shadow: 0 0 0 2px rgba(255, 255, 255, 0.95);
}
</style>
