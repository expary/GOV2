<script setup>
import { onBeforeUnmount, onMounted, ref } from "vue";
import { RefreshCw } from "@lucide/vue";
import { getSystemModules } from "@/api/requests";

const modules = ref([]);
const loading = ref(false);
const error = ref("");
let modulesRequestController = null;
let modulesRequestID = 0;

async function loadModules() {
  modulesRequestController?.abort();
  const controller = new AbortController();
  modulesRequestController = controller;
  const requestID = ++modulesRequestID;
  loading.value = true;
  error.value = "";
  try {
    const result = await getSystemModules({ signal: controller.signal });
    if (requestID !== modulesRequestID) return;
    modules.value = result;
  } catch (err) {
    if (err.name === "AbortError") return;
    if (requestID !== modulesRequestID) return;
    error.value = err.message;
  } finally {
    if (requestID === modulesRequestID) {
      loading.value = false;
      modulesRequestController = null;
    }
  }
}

onMounted(loadModules);

onBeforeUnmount(() => {
  modulesRequestController?.abort();
});
</script>

<template>
  <section class="panel">
    <div class="toolbar end">
      <button class="text-button secondary" type="button" :class="{ busy: loading }" :aria-busy="loading" :disabled="loading" @click="loadModules">
        <RefreshCw :size="18" :class="{ spin: loading }" />
        <span>Refresh</span>
      </button>
    </div>

    <p v-if="error" class="error">
      {{ error }}
    </p>

    <div class="module-grid">
      <article v-for="mod in modules" :key="mod.name" class="module-card">
        <header>
          <div>
            <strong>{{ mod.title }}</strong>
            <span>{{ mod.name }} · {{ mod.version }}</span>
          </div>
          <div class="module-counts">
            <span class="badge">{{ mod.permissions.length }} permissions</span>
            <span class="badge muted">{{ mod.backend_routes.length }} APIs</span>
            <span class="badge muted">{{ mod.menus.length }} menus</span>
            <span class="badge muted">{{ mod.frontend_routes.length }} routes</span>
            <span class="badge muted">{{ mod.migrations.length }} migrations</span>
          </div>
        </header>
        <p class="module-description">
          {{ mod.description }}
        </p>
        <div class="module-sections">
          <section v-if="mod.backend_routes.length" class="module-section">
            <h3>APIs</h3>
            <ul>
              <li v-for="route in mod.backend_routes" :key="route.name">
                <code>{{ route.method }} {{ route.path }}</code>
                <span>{{ route.permission || "auth" }}</span>
              </li>
            </ul>
          </section>
          <section v-if="mod.frontend_routes.length" class="module-section">
            <h3>Routes</h3>
            <ul>
              <li v-for="route in mod.frontend_routes" :key="route.name">
                <code>{{ route.path }}</code>
                <span>{{ route.component }}</span>
              </li>
            </ul>
          </section>
          <section v-if="!mod.backend_routes.length && !mod.frontend_routes.length && mod.permissions.length" class="module-section">
            <h3>Permissions</h3>
            <ul>
              <li v-for="permission in mod.permissions" :key="permission.code">
                <code>{{ permission.code }}</code>
                <span>{{ permission.name }}</span>
              </li>
            </ul>
          </section>
        </div>
      </article>
    </div>
    <div v-if="!loading && modules.length === 0" class="empty-state">
      <p>No modules</p>
    </div>
  </section>
</template>
