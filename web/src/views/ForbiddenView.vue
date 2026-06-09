<script setup>
import { computed } from "vue";
import { RouterLink, useRoute, useRouter } from "vue-router";
import { ArrowLeft, Gauge, ShieldAlert, UserCircle } from "@lucide/vue";
import PermissionGate from "@/components/PermissionGate.vue";
import { permissions } from "@/permissions";
import { useSessionStore } from "@/stores/session";

const route = useRoute();
const router = useRouter();
const session = useSessionStore();

const missingPermission = computed(() => String(route.query.permission || route.meta.permission || ""));
const actor = computed(() => session.user?.nickname || session.user?.username || "Current account");

function goBack() {
  if (window.history.length > 1) {
    router.back();
    return;
  }
  router.push({ name: "account" });
}
</script>

<template>
  <section class="empty-state">
    <ShieldAlert :size="34" />
    <h2>Permission denied</h2>
    <p>{{ actor }} cannot access this page.</p>
    <code v-if="missingPermission" class="empty-meta">{{ missingPermission }}</code>
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
