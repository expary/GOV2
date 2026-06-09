<script setup>
import { computed } from "vue";
import { RouterLink, RouterView, useRoute, useRouter } from "vue-router";
import {
  BookOpen,
  Boxes,
  Gauge,
  History,
  LogOut,
  Menu,
  PanelLeftClose,
  PanelLeftOpen,
  Rows2,
  Rows3,
  SlidersHorizontal,
  Shield,
  UserCircle,
  Users,
} from "@lucide/vue";
import PermissionGate from "@/components/PermissionGate.vue";
import { permissions } from "@/permissions";
import { useAppStore } from "@/stores/app";
import { usePreferencesStore } from "@/stores/preferences";
import { useSessionStore } from "@/stores/session";

const route = useRoute();
const router = useRouter();
const app = useAppStore();
const session = useSessionStore();
const preferences = usePreferencesStore();

const iconMap = {
  "layout-dashboard": Gauge,
  settings: SlidersHorizontal,
  users: Users,
  shield: Shield,
  menu: Menu,
  boxes: Boxes,
  book: BookOpen,
  "sliders-horizontal": SlidersHorizontal,
  history: History,
};

const baseNavGroups = [
  {
    title: "Overview",
    items: [{ label: "Dashboard", to: "/dashboard", icon: Gauge, permission: permissions.dashboardView }],
  },
  {
    title: "System",
    items: [
      { label: "Users", to: "/system/users", icon: Users, permission: permissions.systemUserList },
      { label: "Roles", to: "/system/roles", icon: Shield, permission: permissions.systemRoleList },
      { label: "Menus", to: "/system/menus", icon: Menu, permission: permissions.systemMenuList },
      { label: "Modules", to: "/system/modules", icon: Boxes, permission: permissions.systemModuleList },
      { label: "Dictionaries", to: "/system/dictionaries", icon: BookOpen, permission: permissions.systemDictionaryList },
      { label: "Settings", to: "/system/settings", icon: SlidersHorizontal, permission: permissions.systemSettingList },
      { label: "Audit Logs", to: "/system/audit", icon: History, permission: permissions.systemAuditList },
    ],
  },
];

const title = computed(() => route.meta.title || app.brandTitle);
const brandSubtitle = computed(() => (app.name !== app.brandTitle ? app.name : "System Framework"));
const navGroups = computed(() => {
  const menus = session.profile?.menus;
  if (Array.isArray(menus)) {
    return menuGroups(menus);
  }
  return baseNavGroups
    .map((group) => ({
      ...group,
      items: group.items.filter((item) => session.can(item.permission)),
    }))
    .filter((group) => group.items.length > 0);
});

function menuGroups(menus) {
  const overview = [];
  const groups = [];

  menus.forEach((menu) => {
    const children = menu.children || [];
    if (children.length > 0) {
      const items = children.map(menuItem).filter(Boolean);
      if (items.length > 0) {
        groups.push({ title: menu.title, items });
      }
      return;
    }
    const item = menuItem(menu);
    if (item) {
      overview.push(item);
    }
  });

  return [
    ...(overview.length ? [{ title: "Overview", items: overview }] : []),
    ...groups,
  ];
}

function menuItem(menu) {
  if (!menu.path || menu.hidden) return null;
  if (!session.can(menu.permission)) return null;
  return {
    label: menu.title,
    to: menu.path,
    icon: iconMap[menu.icon] || Menu,
    permission: menu.permission,
  };
}

async function logout() {
  await session.logout();
  router.push({ name: "login" });
}
</script>

<template>
  <main
    class="app-shell"
    :class="{
      'sidebar-collapsed': preferences.sidebarCollapsed,
      'density-compact': preferences.density === 'compact',
    }"
  >
    <aside class="sidebar">
      <div class="sidebar-brand">
        <div class="brand-mark">
          {{ app.brandMark }}
        </div>
        <div>
          <strong>{{ app.brandTitle }}</strong>
          <span>{{ brandSubtitle }}</span>
        </div>
      </div>

      <nav class="nav-groups" aria-label="Main navigation">
        <section v-for="group in navGroups" :key="group.title" class="nav-group">
          <p>{{ group.title }}</p>
          <RouterLink
            v-for="item in group.items"
            :key="item.to"
            :to="item.to"
            class="nav-link"
            active-class="active"
          >
            <component :is="item.icon" :size="18" />
            <span>{{ item.label }}</span>
          </RouterLink>
        </section>
      </nav>
    </aside>

    <section class="workspace">
      <header class="topbar">
        <div class="topbar-title">
          <button
            class="icon-button"
            type="button"
            :title="preferences.sidebarCollapsed ? 'Expand sidebar' : 'Collapse sidebar'"
            @click="preferences.toggleSidebar"
          >
            <component :is="preferences.sidebarCollapsed ? PanelLeftOpen : PanelLeftClose" :size="18" />
          </button>
          <div>
            <h1>{{ title }}</h1>
            <span>{{ session.user?.nickname || session.user?.username }} · {{ session.user?.status }}</span>
          </div>
        </div>
        <div class="topbar-actions">
          <button
            class="icon-button"
            type="button"
            :title="preferences.density === 'compact' ? 'Comfortable density' : 'Compact density'"
            @click="preferences.toggleDensity"
          >
            <component :is="preferences.density === 'compact' ? Rows3 : Rows2" :size="18" />
          </button>
          <RouterLink class="icon-button" to="/account" title="Account">
            <UserCircle :size="18" />
          </RouterLink>
          <PermissionGate :permission="permissions.systemMenuList">
            <RouterLink class="icon-button" to="/system/menus" title="Menus">
              <Menu :size="18" />
            </RouterLink>
          </PermissionGate>
          <PermissionGate :permission="permissions.systemModuleList">
            <RouterLink class="icon-button" to="/system/modules" title="Modules">
              <Boxes :size="18" />
            </RouterLink>
          </PermissionGate>
          <PermissionGate :permission="permissions.systemDictionaryList">
            <RouterLink class="icon-button" to="/system/dictionaries" title="Dictionaries">
              <BookOpen :size="18" />
            </RouterLink>
          </PermissionGate>
          <PermissionGate :permission="permissions.systemSettingList">
            <RouterLink class="icon-button" to="/system/settings" title="Settings">
              <SlidersHorizontal :size="18" />
            </RouterLink>
          </PermissionGate>
          <button class="text-button" type="button" @click="logout">
            <LogOut :size="18" />
            <span>Logout</span>
          </button>
        </div>
      </header>

      <RouterView />
    </section>
  </main>
</template>
