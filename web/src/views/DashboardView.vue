<script setup>
import { onBeforeUnmount, onMounted, ref } from "vue";
import { Activity, ClipboardList, Shield, Users } from "@lucide/vue";
import { getDashboardSummary } from "@/api/requests";

const loading = ref(true);
const error = ref("");
const summary = ref({
  user_count: 0,
  active_user_count: 0,
  role_count: 0,
  audit_log_count: 0,
});
let summaryRequestController = null;
let summaryRequestID = 0;

const cards = [
  { label: "Users", key: "user_count", icon: Users },
  { label: "Active Users", key: "active_user_count", icon: Activity },
  { label: "Roles", key: "role_count", icon: Shield },
  { label: "Audit Logs", key: "audit_log_count", icon: ClipboardList },
];

async function loadSummary() {
  summaryRequestController?.abort();
  const controller = new AbortController();
  summaryRequestController = controller;
  const requestID = ++summaryRequestID;
  loading.value = true;
  error.value = "";
  try {
    const result = await getDashboardSummary({ signal: controller.signal });
    if (requestID !== summaryRequestID) return;
    summary.value = result;
  } catch (err) {
    if (err.name === "AbortError") return;
    if (requestID !== summaryRequestID) return;
    error.value = err.message;
  } finally {
    if (requestID === summaryRequestID) {
      loading.value = false;
      summaryRequestController = null;
    }
  }
}

onMounted(loadSummary);

onBeforeUnmount(() => {
  summaryRequestController?.abort();
});
</script>

<template>
  <p v-if="error" class="error">
    {{ error }}
  </p>
  <section class="summary-grid" :aria-busy="loading">
    <article v-for="card in cards" :key="card.key" class="metric">
      <component :is="card.icon" :size="22" />
      <span>{{ card.label }}</span>
      <strong>{{ loading ? "-" : summary[card.key] }}</strong>
    </article>
  </section>
</template>
