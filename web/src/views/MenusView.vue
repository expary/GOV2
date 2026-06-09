<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { Pencil, Plus, RefreshCw, Save, Trash2, X } from "@lucide/vue";
import { validationErrorsByField } from "@/api/client";
import {
  deleteSystemMenusId,
  getSystemMenus,
  postSystemMenus,
  putSystemMenusId,
} from "@/api/requests";
import PermissionButton from "@/components/PermissionButton.vue";
import { permissions } from "@/permissions";
import { useSessionStore } from "@/stores/session";

const menus = ref([]);
const loading = ref(false);
const saving = ref(false);
const editingID = ref(null);
const error = ref("");
const fieldErrors = ref({});
const form = ref(emptyForm());
const session = useSessionStore();
let menusRequestController = null;
let menusRequestID = 0;

const flatMenus = computed(() => flattenMenus(menus.value));
const parentOptions = computed(() => flatMenus.value.filter((menu) => menu.id !== editingID.value));
const showForm = computed(() => session.can(permissions.systemMenuCreate) || editingID.value !== null);
const canSave = computed(() => {
  const permission = editingID.value ? permissions.systemMenuUpdate : permissions.systemMenuCreate;
  return session.can(permission);
});

function emptyForm() {
  return {
    parent_id: 0,
    title: "",
    name: "",
    path: "",
    icon: "",
    component: "",
    permission: "",
    sort: 0,
    hidden: false,
  };
}

async function loadMenus() {
  menusRequestController?.abort();
  const controller = new AbortController();
  menusRequestController = controller;
  const requestID = ++menusRequestID;
  loading.value = true;
  error.value = "";
  try {
    const result = await getSystemMenus({ signal: controller.signal });
    if (requestID !== menusRequestID) return;
    menus.value = result;
  } catch (err) {
    if (err.name === "AbortError") return;
    if (requestID !== menusRequestID) return;
    error.value = err.message;
  } finally {
    if (requestID === menusRequestID) {
      loading.value = false;
      menusRequestController = null;
    }
  }
}

async function reloadProfileMenus() {
  try {
    await session.loadProfile();
  } catch {
    // Global auth-expired handling owns session cleanup.
  }
}

function flattenMenus(items, level = 0) {
  return items.flatMap((item) => [
    { ...item, level },
    ...flattenMenus(item.children || [], level + 1),
  ]);
}

function startCreate() {
  editingID.value = null;
  form.value = emptyForm();
  clearFormErrors();
}

function startEdit(menu) {
  editingID.value = menu.id;
  form.value = {
    parent_id: menu.parent_id || 0,
    title: menu.title,
    name: menu.name,
    path: menu.path,
    icon: menu.icon || "",
    component: menu.component || "",
    permission: menu.permission || "",
    sort: menu.sort || 0,
    hidden: Boolean(menu.hidden),
  };
  clearFormErrors();
}

function cancelEdit() {
  editingID.value = null;
  form.value = emptyForm();
  clearFormErrors();
}

function clearFormErrors() {
  error.value = "";
  fieldErrors.value = {};
}

function fieldError(field) {
  return fieldErrors.value[field] || "";
}

async function saveMenu() {
  saving.value = true;
  clearFormErrors();
  try {
    const payload = {
      ...form.value,
      parent_id: Number(form.value.parent_id) || 0,
      sort: Number(form.value.sort) || 0,
    };
    if (editingID.value) {
      await putSystemMenusId({
        params: { id: editingID.value },
        body: payload,
      });
    } else {
      await postSystemMenus({
        body: payload,
      });
    }
    cancelEdit();
    await loadMenus();
    await reloadProfileMenus();
  } catch (err) {
    fieldErrors.value = validationErrorsByField(err);
    error.value = err.message;
  } finally {
    saving.value = false;
  }
}

async function deleteMenu(menu) {
  if (!window.confirm(`Delete menu "${menu.title}"?`)) return;
  loading.value = true;
  error.value = "";
  try {
    await deleteSystemMenusId({ params: { id: menu.id } });
    if (editingID.value === menu.id) {
      cancelEdit();
    }
    await loadMenus();
    await reloadProfileMenus();
  } catch (err) {
    error.value = err.message;
  } finally {
    loading.value = false;
  }
}

onMounted(loadMenus);

onBeforeUnmount(() => {
  menusRequestController?.abort();
});
</script>

<template>
  <section class="panel">
    <div class="toolbar">
      <PermissionButton class="text-button" :permission="permissions.systemMenuCreate" @click="startCreate">
        <Plus :size="18" />
        <span>New menu</span>
      </PermissionButton>
      <button class="text-button secondary" type="button" :class="{ busy: loading }" :aria-busy="loading" :disabled="loading" @click="loadMenus">
        <RefreshCw :size="18" :class="{ spin: loading }" />
        <span>Refresh</span>
      </button>
    </div>

    <form v-if="showForm" class="inline-form" @submit.prevent="saveMenu">
      <label>
        <span>Parent</span>
        <select v-model.number="form.parent_id">
          <option :value="0">Root</option>
          <option v-for="menu in parentOptions" :key="menu.id" :value="menu.id">
            {{ `${"- ".repeat(menu.level)}${menu.title}` }}
          </option>
        </select>
        <small v-if="fieldError('parent_id')" class="field-error">{{ fieldError("parent_id") }}</small>
      </label>
      <label>
        <span>Title</span>
        <input v-model.trim="form.title" required placeholder="Users">
        <small v-if="fieldError('title')" class="field-error">{{ fieldError("title") }}</small>
      </label>
      <label>
        <span>Name</span>
        <input v-model.trim="form.name" required placeholder="system-users">
        <small v-if="fieldError('name')" class="field-error">{{ fieldError("name") }}</small>
      </label>
      <label>
        <span>Path</span>
        <input v-model.trim="form.path" required placeholder="/system/users">
        <small v-if="fieldError('path')" class="field-error">{{ fieldError("path") }}</small>
      </label>
      <label>
        <span>Icon</span>
        <input v-model.trim="form.icon" placeholder="users">
      </label>
      <label>
        <span>Component</span>
        <input v-model.trim="form.component" placeholder="UsersView">
      </label>
      <label>
        <span>Permission</span>
        <input v-model.trim="form.permission" placeholder="system:user:list">
      </label>
      <label>
        <span>Sort</span>
        <input v-model.number="form.sort" type="number" step="1">
      </label>
      <label class="check-field">
        <input v-model="form.hidden" type="checkbox">
        <span>Hidden</span>
      </label>
      <div class="form-actions">
        <button class="text-button" type="submit" :class="{ busy: saving }" :aria-busy="saving" :disabled="saving || !canSave">
          <Save :size="18" />
          <span>{{ editingID ? "Save" : "Create" }}</span>
        </button>
        <button v-if="editingID" class="text-button secondary" type="button" @click="cancelEdit">
          <X :size="18" />
          <span>Cancel</span>
        </button>
      </div>
      <p v-if="error" class="error">
        {{ error }}
      </p>
    </form>

    <p v-if="error && !showForm" class="error">
      {{ error }}
    </p>

    <div class="table-wrap">
      <table>
        <thead>
          <tr>
            <th>ID</th>
            <th>Title</th>
            <th>Path</th>
            <th>Component</th>
            <th>Permission</th>
            <th>Sort</th>
            <th>Hidden</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="menu in flatMenus" :key="menu.id">
            <td>{{ menu.id }}</td>
            <td>
              <span :style="{ paddingLeft: `${menu.level * 18}px` }">{{ menu.title }}</span>
            </td>
            <td>{{ menu.path }}</td>
            <td>{{ menu.component }}</td>
            <td>{{ menu.permission || "-" }}</td>
            <td>{{ menu.sort }}</td>
            <td>
              <span class="badge" :class="{ muted: !menu.hidden }">{{ menu.hidden ? "yes" : "no" }}</span>
            </td>
            <td>
              <div class="row-actions">
                <PermissionButton class="icon-button" :permission="permissions.systemMenuUpdate" title="Edit" @click="startEdit(menu)">
                  <Pencil :size="17" />
                </PermissionButton>
                <PermissionButton class="icon-button danger" :permission="permissions.systemMenuDelete" title="Delete" @click="deleteMenu(menu)">
                  <Trash2 :size="17" />
                </PermissionButton>
              </div>
            </td>
          </tr>
          <tr v-if="!loading && flatMenus.length === 0">
            <td colspan="8">
              <div class="empty-table">
                No menus
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>
