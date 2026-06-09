<script setup>
import { computed } from "vue";
import { RouterLink, useRoute, useRouter } from "vue-router";
import { ArrowLeft, Gauge, TriangleAlert, UserCircle } from "@lucide/vue";
import PermissionGate from "@/components/PermissionGate.vue";
import { permissions } from "@/permissions";

const route = useRoute();
const router = useRouter();
const missingPath = computed(() => route.fullPath || route.path);

function goBack() {
  if (window.history.length > 1) {
    router.back();
    return;
  }
  router.push({ name: "account" });
}
</script>

<template>
  <section class="empty-state not-found-state">
    <TriangleAlert :size="34" />
    <h2>Page not found</h2>
    <p>The requested admin route does not exist.</p>
    <code class="empty-meta">{{ missingPath }}</code>
    <div class="empty-actions">
      <button class="text-button secondary" type="button" @click="goBack">
        <ArrowLeft :size="18" />
        <span>Back</span>
      </button>
      <PermissionGate :permission="permissions.dashboardView">
        <RouterLink class="text-button" to="/dashboard">
          <Gauge :size="18" />
          <span>Dashboard</span>
        </RouterLink>
      </PermissionGate>
      <RouterLink class="text-button secondary" to="/account">
        <UserCircle :size="18" />
        <span>Account</span>
      </RouterLink>
    </div>
  </section>
</template>
