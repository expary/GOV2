import { defineStore } from "pinia";

const sidebarKey = "gov2_sidebar_collapsed";
const densityKey = "gov2_density";
const densities = new Set(["comfortable", "compact"]);

function storedBoolean(key, fallback) {
  const value = localStorage.getItem(key);
  if (value === null) return fallback;
  return value === "true";
}

function storedDensity() {
  const value = localStorage.getItem(densityKey);
  return densities.has(value) ? value : "comfortable";
}

export const usePreferencesStore = defineStore("preferences", {
  state: () => ({
    sidebarCollapsed: storedBoolean(sidebarKey, false),
    density: storedDensity(),
  }),
  actions: {
    toggleSidebar() {
      this.sidebarCollapsed = !this.sidebarCollapsed;
      localStorage.setItem(sidebarKey, String(this.sidebarCollapsed));
    },
    setDensity(density) {
      if (!densities.has(density)) return;
      this.density = density;
      localStorage.setItem(densityKey, density);
    },
    toggleDensity() {
      this.setDensity(this.density === "compact" ? "comfortable" : "compact");
    },
  },
});
