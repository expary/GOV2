<script setup>
import { computed, onMounted, watch } from "vue";
import { RouterView, useRoute } from "vue-router";
import { useAppStore } from "@/stores/app";

const route = useRoute();
const app = useAppStore();

const documentTitle = computed(() => {
  const pageTitle = route.meta.title;
  return pageTitle ? `${pageTitle} · ${app.brandTitle}` : app.brandTitle;
});

function syncDocumentTitle() {
  document.title = documentTitle.value;
}

watch(documentTitle, syncDocumentTitle, { immediate: true });

onMounted(async () => {
  await app.loadConfig();
  syncDocumentTitle();
});
</script>

<template>
  <RouterView />
</template>
