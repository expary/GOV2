import { defineStore } from "pinia";
import { getAppConfig } from "@/api/requests";

let configRequest = null;

export const useAppStore = defineStore("app", {
  state: () => ({
    name: "GOV2",
    title: "GOV2",
    environment: "",
    loaded: false,
    error: "",
  }),
  getters: {
    brandTitle: (state) => cleanText(state.title, cleanText(state.name, "GOV2")),
    brandMark: (state) => initials(cleanText(state.title, state.name)),
  },
  actions: {
    async loadConfig() {
      if (this.loaded) {
        return this.snapshot();
      }
      if (configRequest) {
        return configRequest;
      }

      configRequest = getAppConfig()
        .then((config) => {
          this.name = cleanText(config?.name, "GOV2");
          this.title = cleanText(config?.title, this.name);
          this.environment = cleanText(config?.environment, "");
          this.loaded = true;
          this.error = "";
          return this.snapshot();
        })
        .catch((err) => {
          this.loaded = true;
          this.error = err.message || "Configuration unavailable";
          return this.snapshot();
        })
        .finally(() => {
          configRequest = null;
        });

      return configRequest;
    },
    snapshot() {
      return {
        name: this.name,
        title: this.title,
        environment: this.environment,
      };
    },
  },
});

function cleanText(value, fallback) {
  const text = typeof value === "string" ? value.trim() : "";
  return text || fallback;
}

function initials(value) {
  const text = cleanText(value, "GOV2");
  const words = text.split(/\s+/).filter(Boolean);
  if (words.length >= 2) {
    return `${words[0][0]}${words[1][0]}`.toUpperCase();
  }
  return text.slice(0, 2).toUpperCase();
}
