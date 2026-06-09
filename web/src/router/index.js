import { createRouter, createWebHistory } from "vue-router";
import AdminLayout from "@/layouts/AdminLayout.vue";
import LoginView from "@/views/LoginView.vue";
import DashboardView from "@/views/DashboardView.vue";
import UsersView from "@/views/UsersView.vue";
import RolesView from "@/views/RolesView.vue";
import MenusView from "@/views/MenusView.vue";
import ModulesView from "@/views/ModulesView.vue";
import DictionariesView from "@/views/DictionariesView.vue";
import SettingsView from "@/views/SettingsView.vue";
import AuditLogsView from "@/views/AuditLogsView.vue";
import AccountView from "@/views/AccountView.vue";
import ForbiddenView from "@/views/ForbiddenView.vue";
import NotFoundView from "@/views/NotFoundView.vue";
import { permissions } from "@/permissions";
import { useSessionStore } from "@/stores/session";

const routes = [
  {
    path: "/login",
    name: "login",
    component: LoginView,
  },
  {
    path: "/",
    component: AdminLayout,
    meta: { requiresAuth: true },
    children: [
      { path: "", redirect: "/dashboard" },
      { path: "403", name: "forbidden", component: ForbiddenView, meta: { title: "Forbidden" } },
      { path: "account", name: "account", component: AccountView, meta: { title: "Account" } },
      { path: "dashboard", name: "dashboard", component: DashboardView, meta: { title: "Dashboard", permission: permissions.dashboardView } },
      { path: "system/users", name: "users", component: UsersView, meta: { title: "Users", permission: permissions.systemUserList } },
      { path: "system/roles", name: "roles", component: RolesView, meta: { title: "Roles", permission: permissions.systemRoleList } },
      { path: "system/menus", name: "menus", component: MenusView, meta: { title: "Menus", permission: permissions.systemMenuList } },
      { path: "system/modules", name: "modules", component: ModulesView, meta: { title: "Modules", permission: permissions.systemModuleList } },
      { path: "system/dictionaries", name: "dictionaries", component: DictionariesView, meta: { title: "Dictionaries", permission: permissions.systemDictionaryList } },
      { path: "system/settings", name: "settings", component: SettingsView, meta: { title: "Settings", permission: permissions.systemSettingList } },
      { path: "system/audit", name: "audit", component: AuditLogsView, meta: { title: "Audit Logs", permission: permissions.systemAuditList } },
      { path: ":pathMatch(.*)*", name: "not-found", component: NotFoundView, meta: { title: "Not Found" } },
    ],
  },
];

const router = createRouter({
  history: createWebHistory(),
  routes,
});

router.beforeEach(async (to) => {
  const session = useSessionStore();
  if (to.meta.requiresAuth && !session.token) {
    return { name: "login", query: { redirect: to.fullPath } };
  }
  if (to.meta.requiresAuth && !session.profile) {
    try {
      await session.loadProfile();
    } catch {
      session.clear();
      return { name: "login", query: { redirect: to.fullPath } };
    }
  }
  if (to.meta.requiresAuth && !session.can(to.meta.permission)) {
    return {
      name: "forbidden",
      query: to.meta.permission ? { permission: to.meta.permission } : undefined,
    };
  }
  if (to.name === "login" && session.token) {
    return { name: "dashboard" };
  }
  return true;
});

export default router;
