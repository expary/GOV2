<script setup>
import { computed, useAttrs } from "vue";
import { useSessionStore } from "@/stores/session";

const props = defineProps({
  permission: {
    type: String,
    default: "",
  },
  mode: {
    type: String,
    default: "hide",
    validator: (value) => ["hide", "disable"].includes(value),
  },
  type: {
    type: String,
    default: "button",
  },
});

const attrs = useAttrs();
const session = useSessionStore();
const allowed = computed(() => session.can(props.permission));
const visible = computed(() => allowed.value || props.mode === "disable");
const disabled = computed(() => Boolean(attrs.disabled) || (!allowed.value && props.mode === "disable"));
</script>

<template>
  <button v-if="visible" v-bind="attrs" :type="type" :disabled="disabled">
    <slot />
  </button>
</template>
