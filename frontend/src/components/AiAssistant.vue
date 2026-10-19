<script setup lang="ts">
import { ref, nextTick, onMounted, watch } from "vue";
import { useI18n } from "vue-i18n";
import { useMainStore } from "../stores/main";
import OverlayMotion from "@/components/base/OverlayMotion.vue";

const store = useMainStore();
const { t } = useI18n();

interface ChatMsg {
  role: "user" | "ai";
  text: string;
}

const showDrawer = ref(false);
const messages = ref<ChatMsg[]>([
  { role: "ai", text: t("common.yunqi.ai.hello") },
]);
const inputText = ref("");
const sending = ref(false);
const listRef = ref<HTMLDivElement | null>(null);

const scrollToBottom = () => {
  nextTick(() => {
    const el = listRef.value;
    if (el) el.scrollTop = el.scrollHeight;
  });
};

const send = async () => {
  const text = inputText.value.trim();
  if (!text || sending.value) return;
  if (!store.isLogged) {
    messages.value.push({ role: "ai", text: t("common.yunqi.ai.loginFirst") });
    inputText.value = "";
    scrollToBottom();
    return;
  }
  messages.value.push({ role: "user", text });
  inputText.value = "";
  sending.value = true;
  scrollToBottom();
  try {
    const headers: Record<string, string> = { "Content-Type": "application/json" };
    const token = localStorage.getItem("flat-nas-token");
    if (token) headers["Authorization"] = `Bearer ${token}`;
    const res = await fetch("/api/ai/chat", {
      method: "POST",
      headers,
      body: JSON.stringify({ message: text }),
    });
    if (!res.ok) throw new Error(`服务器返回 ${res.status}`);
    const data = await res.json();
    const reply = String(data?.reply ?? "").trim();
    messages.value.push({
      role: "ai",
      text: reply || t("common.yunqi.ai.emptyReply"),
    });
  } catch (e) {
    console.error("AI 聊天失败", e);
    messages.value.push({
      role: "ai",
      text: t("common.yunqi.ai.error"),
    });
  } finally {
    sending.value = false;
    scrollToBottom();
  }
};

const clearChat = () => {
  messages.value = [
    { role: "ai", text: t("common.yunqi.ai.cleared") },
  ];
};

onMounted(() => {
  scrollToBottom();
});
watch(showDrawer, (v) => {
  if (v) scrollToBottom();
});
</script>

<template>
  <!-- 右下角悬浮按钮 -->
  <button
    @click="showDrawer = !showDrawer"
    class="fixed bottom-20 right-6 z-[100] w-12 h-12 rounded-full bg-white/20 backdrop-blur-md border border-white/30 text-white shadow-lg flex items-center justify-center hover:bg-white/40 active:scale-95 transition-all text-2xl"
    :title="t('common.yunqi.ai.title')"
    :aria-label="t('common.yunqi.ai.title')"
  >
    {{ showDrawer ? "✕" : "🤖" }}
  </button>

  <OverlayMotion
    :show="showDrawer"
    :z-index="9990"
    variant="popover"
    panel-class="fixed bottom-[132px] right-6 w-[min(380px,calc(100vw-48px))] h-[min(520px,60vh)] bg-white rounded-2xl border border-gray-200 shadow-2xl flex flex-col overflow-hidden text-gray-800"
  >
    <div class="px-4 py-3 border-b border-gray-100 flex items-center justify-between bg-gray-50/80">
      <div class="text-sm font-bold text-gray-900 flex items-center gap-2">🤖 {{ t("common.yunqi.ai.title") }}</div>
      <div class="flex items-center gap-2">
        <button
          @click="clearChat"
          class="text-xs text-gray-500 hover:text-gray-900 px-2 py-1 rounded hover:bg-gray-200"
        >
          {{ t("common.yunqi.ai.clear") }}
        </button>
        <button @click="showDrawer = false" class="text-gray-400 hover:text-gray-700 text-lg leading-none">
          ×
        </button>
      </div>
    </div>

    <div ref="listRef" class="flex-1 overflow-y-auto p-3 space-y-2.5 bg-white">
      <div
        v-for="(m, i) in messages"
        :key="i"
        class="flex"
        :class="m.role === 'user' ? 'justify-end' : 'justify-start'"
      >
        <div
          class="max-w-[85%] px-3 py-2 rounded-2xl text-sm leading-relaxed whitespace-pre-wrap break-words"
          :class="
            m.role === 'user'
              ? 'bg-gray-900 text-white rounded-br-md'
              : 'bg-gray-100 text-gray-800 rounded-bl-md'
          "
        >
          {{ m.text }}
        </div>
      </div>
      <div v-if="sending" class="flex justify-start">
        <div class="px-3 py-2 rounded-2xl rounded-bl-md bg-gray-100 text-sm text-gray-500 flex items-center gap-2">
          <span class="w-3 h-3 border-2 border-gray-300 border-t-gray-600 rounded-full animate-spin"></span>
          {{ t("common.yunqi.ai.thinking") }}
        </div>
      </div>
    </div>

    <div class="p-3 border-t border-gray-100 bg-gray-50/80 flex gap-2">
      <input
        v-model="inputText"
        :placeholder="t('common.yunqi.ai.inputPh')"
        class="flex-1 min-w-0 text-sm px-3 py-2 rounded-xl border border-gray-200 focus:border-gray-900 outline-none"
        @keyup.enter="send"
        :disabled="sending"
      />
      <button
        @click="send"
        :disabled="sending || !inputText.trim()"
        class="text-sm bg-gray-900 text-white px-4 py-2 rounded-xl font-bold hover:bg-black disabled:opacity-40 transition-all"
      >
        {{ t("common.yunqi.ai.send") }}
      </button>
    </div>
  </OverlayMotion>
</template>
