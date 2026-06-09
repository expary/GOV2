<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { ChevronLeft, ChevronRight, RefreshCw, Search, X } from "@lucide/vue";
import { getSystemAuditLogs } from "@/api/requests";

const logs = ref([]);
const loading = ref(false);
const error = ref("");
const total = ref(0);
const page = ref(1);
const pageSize = ref(20);
const filters = ref({
  keyword: "",
  actor: "",
  action: "",
  resource: "",
  resource_id: "",
});
let logsRequestController = null;
let logsRequestID = 0;
const actionOptions = [
  "create",
  "update",
  "delete",
  "status",
  "login",
  "login_failed",
  "logout",
  "password",
  "password_failed",
  "bootstrap_admin",
  "reset_admin_password",
  "seed",
];

const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)));
const pageStart = computed(() => (total.value === 0 ? 0 : (page.value - 1) * pageSize.value + 1));
const pageEnd = computed(() => Math.min(total.value, page.value * pageSize.value));

async function loadLogs(nextPage = page.value) {
  logsRequestController?.abort();
  const controller = new AbortController();
  logsRequestController = controller;
  const requestID = ++logsRequestID;
  loading.value = true;
  error.value = "";
  page.value = nextPage;
  try {
    const result = await getSystemAuditLogs({
      query: {
        page: page.value,
        page_size: pageSize.value,
        keyword: filters.value.keyword.trim(),
        actor: filters.value.actor.trim(),
        action: filters.value.action,
        resource: filters.value.resource.trim(),
        resource_id: filters.value.resource_id.trim(),
      },
      signal: controller.signal,
    });
    if (requestID !== logsRequestID) return;
    logs.value = result.items || [];
    total.value = result.total || 0;
    page.value = result.page || page.value;
    pageSize.value = result.page_size || pageSize.value;
  } catch (err) {
    if (err.name === "AbortError") return;
    error.value = err.message;
  } finally {
    if (requestID === logsRequestID) {
      loading.value = false;
      logsRequestController = null;
    }
  }
}

function applyFilters() {
  loadLogs(1);
}

function resetFilters() {
  filters.value = {
    keyword: "",
    actor: "",
    action: "",
    resource: "",
    resource_id: "",
  };
  loadLogs(1);
}

function formatTime(value) {
  if (!value) return "-";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? "-" : date.toLocaleString();
}

onMounted(loadLogs);

onBeforeUnmount(() => {
  logsRequestController?.abort();
});
</script>

<template>
  <section class="panel">
    <form class="toolbar filter-toolbar" @submit.prevent="applyFilters">
      <label class="search-input">
        <Search :size="18" />
        <input v-model="filters.keyword" placeholder="Keyword">
      </label>
      <input v-model="filters.actor" class="filter-input" placeholder="Actor">
      <select v-model="filters.action" class="filter-select">
        <option value="">
          All actions
        </option>
        <option v-for="action in actionOptions" :key="action" :value="action">
          {{ action }}
        </option>
      </select>
      <input v-model="filters.resource" class="filter-input" placeholder="Resource">
      <input v-model="filters.resource_id" class="filter-input" placeholder="Resource ID">
      <select v-model="pageSize" class="filter-select" @change="applyFilters">
        <option :value="20">
          20 / page
        </option>
        <option :value="50">
          50 / page
        </option>
        <option :value="100">
          100 / page
        </option>
      </select>
      <button class="text-button" type="submit" :class="{ busy: loading }" :aria-busy="loading" :disabled="loading">
        <Search :size="18" />
        <span>Search</span>
      </button>
      <button class="text-button secondary" type="button" :class="{ busy: loading }" :aria-busy="loading" :disabled="loading" @click="resetFilters">
        <X :size="18" />
        <span>Reset</span>
      </button>
      <button class="text-button secondary" type="button" :class="{ busy: loading }" :aria-busy="loading" :disabled="loading" @click="loadLogs()">
        <RefreshCw :size="18" :class="{ spin: loading }" />
        <span>Refresh</span>
      </button>
    </form>

    <p v-if="error" class="error">
      {{ error }}
    </p>

    <div class="table-wrap">
      <table>
        <thead>
          <tr>
            <th>ID</th>
            <th>Actor</th>
            <th>Action</th>
            <th>Resource</th>
            <th>Resource ID</th>
            <th>Detail</th>
            <th>IP</th>
            <th>User Agent</th>
            <th>Created At</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="log in logs" :key="log.id">
            <td>{{ log.id }}</td>
            <td>{{ log.actor }}</td>
            <td>{{ log.action }}</td>
            <td>{{ log.resource }}</td>
            <td>{{ log.resource_id || "-" }}</td>
            <td class="wrap-cell">
              {{ log.detail || "-" }}
            </td>
            <td>{{ log.ip || "-" }}</td>
            <td class="wrap-cell">
              {{ log.user_agent || "-" }}
            </td>
            <td>{{ formatTime(log.created_at) }}</td>
          </tr>
          <tr v-if="!loading && logs.length === 0">
            <td colspan="9">
              <div class="empty-table">
                No audit logs
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <div class="table-footer">
      <span>{{ pageStart }}-{{ pageEnd }} of {{ total }}</span>
      <div class="row-actions">
        <button class="icon-button" type="button" title="Previous page" :disabled="loading || page <= 1" @click="loadLogs(page - 1)">
          <ChevronLeft :size="18" />
        </button>
        <span class="page-indicator">{{ page }} / {{ totalPages }}</span>
        <button class="icon-button" type="button" title="Next page" :disabled="loading || page >= totalPages" @click="loadLogs(page + 1)">
          <ChevronRight :size="18" />
        </button>
      </div>
    </div>
  </section>
</template>
