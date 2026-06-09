<script setup>
import { computed } from "vue";
import { useSessionStore } from "@/stores/session";

const props = defineProps({
  permission: {
    type: String,
    default: "",
  },
});

const session = useSessionStore();
const allowed = computed(() => session.can(props.permission));
</script>

<template>
  <slot v-if="allowed" />
</template>
