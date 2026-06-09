import { createApp } from "vue";
import { createPinia } from "pinia";
import App from "./App.vue";
import router from "./router";
import { useSessionStore } from "@/stores/session";
import "./styles.css";

const pinia = createPinia();

window.addEventListener("gov2:auth-expired", () => {
  const session = useSessionStore();
  session.clear();
  const current = router.currentRoute.value;
  if (current.name !== "login") {
    router.push({ name: "login", query: { redirect: current.fullPath } });
  }
});

createApp(App).use(pinia).use(router).mount("#app");
