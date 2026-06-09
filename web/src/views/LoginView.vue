<script setup>
import { computed, reactive, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import { LogIn } from "@lucide/vue";
import { useAppStore } from "@/stores/app";
import { useSessionStore } from "@/stores/session";

const router = useRouter();
const route = useRoute();
const app = useAppStore();
const session = useSessionStore();
const loading = ref(false);
const error = ref("");
const form = reactive({
  username: "admin",
  password: "admin123",
});
const brandTitle = computed(() => app.brandTitle);
const brandMark = computed(() => app.brandMark);

async function submit() {
  error.value = "";
  loading.value = true;
  try {
    await session.login({
      username: form.username.trim(),
      password: form.password,
    });
    router.push(route.query.redirect || "/dashboard");
  } catch (err) {
    error.value = err.message || "Login failed";
  } finally {
    loading.value = false;
  }
}
</script>

<template>
  <main class="login-shell">
    <section class="login-panel">
      <div class="brand-mark">
        {{ brandMark }}
      </div>
      <h1>{{ brandTitle }}</h1>
      <form class="form" @submit.prevent="submit">
        <label>
          <span>Username</span>
          <input v-model="form.username" autocomplete="username">
        </label>
        <label>
          <span>Password</span>
          <input v-model="form.password" type="password" autocomplete="current-password">
        </label>
        <button type="submit" :class="{ busy: loading }" :aria-busy="loading" :disabled="loading">
          <LogIn :size="18" />
          <span>{{ loading ? "Signing in" : "Sign in" }}</span>
        </button>
        <p v-if="error" class="error">
          {{ error }}
        </p>
      </form>
    </section>
  </main>
</template>
