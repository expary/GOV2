<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { Pencil, Plus, RefreshCw, Save, Trash2, X } from "@lucide/vue";
import { validationErrorsByField } from "@/api/client";
import {
  deleteSystemRolesId,
  getSystemPermissions,
  getSystemRoles,
  postSystemRoles,
  putSystemRolesId,
} from "@/api/requests";
import PermissionButton from "@/components/PermissionButton.vue";
import { permissions } from "@/permissions";
import { useSessionStore } from "@/stores/session";

const roles = ref([]);
const permissionCatalog = ref([]);
const loading = ref(false);
const saving = ref(false);
const editingID = ref(null);
const error = ref("");
const fieldErrors = ref({});
const form = ref(emptyForm());
const session = useSessionStore();
let rolesRequestController = null;
let rolesRequestID = 0;
let permissionsRequestController = null;
let permissionsRequestID = 0;

const groupedPermissions = computed(() => {
  const groups = new Map();
  permissionCatalog.value.forEach((permission) => {
    const module = permission.module || "custom";
    if (!groups.has(module)) groups.set(module, []);
    groups.get(module).push(permission);
  });
  return Array.from(groups.entries()).map(([module, items]) => ({
    module,
    items: items.sort((a, b) => a.code.localeCompare(b.code)),
  }));
});
const showForm = computed(() => session.can(permissions.systemRoleCreate) || editingID.value !== null);
const canSave = computed(() => {
  const permission = editingID.value ? permissions.systemRoleUpdate : permissions.systemRoleCreate;
  return session.can(permission);
});

function emptyForm() {
  return {
    name: "",
    code: "",
    description: "",
    permissions: [],
  };
}

async function loadRoles() {
  rolesRequestController?.abort();
  const controller = new AbortController();
  rolesRequestController = controller;
  const requestID = ++rolesRequestID;
  loading.value = true;
  error.value = "";
  try {
    const result = await getSystemRoles({ signal: controller.signal });
    if (requestID !== rolesRequestID) return;
    roles.value = result;
  } catch (err) {
    if (err.name === "AbortError") return;
    if (requestID !== rolesRequestID) return;
    error.value = err.message;
  } finally {
    if (requestID === rolesRequestID) {
      loading.value = false;
      rolesRequestController = null;
    }
  }
}

async function loadPermissions() {
  permissionsRequestController?.abort();
  const controller = new AbortController();
  permissionsRequestController = controller;
  const requestID = ++permissionsRequestID;
  try {
    const result = await getSystemPermissions({ signal: controller.signal });
    if (requestID !== permissionsRequestID) return;
    permissionCatalog.value = result;
  } catch (err) {
    if (err.name === "AbortError") return;
    if (requestID !== permissionsRequestID) return;
    error.value = err.message;
  } finally {
    if (requestID === permissionsRequestID) {
      permissionsRequestController = null;
    }
  }
}

function startCreate() {
  editingID.value = null;
  form.value = emptyForm();
  clearFormErrors();
}

function startEdit(role) {
  editingID.value = role.id;
  form.value = {
    name: role.name,
    code: role.code,
    description: role.description || "",
    permissions: [...(role.permissions || [])],
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

function togglePermission(code, checked) {
  const current = new Set(form.value.permissions);
  if (checked) {
    if (code === permissions.all) {
      form.value.permissions = [permissions.all];
      return;
    }
    current.delete(permissions.all);
    current.add(code);
  } else {
    current.delete(code);
  }
  form.value.permissions = Array.from(current).sort();
}

function hasPermission(code) {
  return form.value.permissions.includes(code);
}

async function saveRole() {
  saving.value = true;
  clearFormErrors();
  try {
    const payload = {
      ...form.value,
      permissions: [...form.value.permissions],
    };
    if (editingID.value) {
      await putSystemRolesId({
        params: { id: editingID.value },
        body: payload,
      });
    } else {
      await postSystemRoles({
        body: payload,
      });
    }
    cancelEdit();
    await loadRoles();
  } catch (err) {
    fieldErrors.value = validationErrorsByField(err);
    error.value = err.message;
  } finally {
    saving.value = false;
  }
}

async function deleteRole(role) {
  if (!window.confirm(`Delete role "${role.name}"?`)) return;
  loading.value = true;
  error.value = "";
  try {
    await deleteSystemRolesId({ params: { id: role.id } });
    if (editingID.value === role.id) {
      cancelEdit();
    }
    await loadRoles();
  } catch (err) {
    error.value = err.message;
  } finally {
    loading.value = false;
  }
}

onMounted(async () => {
  await Promise.allSettled([loadRoles(), loadPermissions()]);
});

onBeforeUnmount(() => {
  rolesRequestController?.abort();
  permissionsRequestController?.abort();
});
</script>

<template>
  <section class="panel">
    <div class="toolbar">
      <PermissionButton class="text-button" :permission="permissions.systemRoleCreate" @click="startCreate">
        <Plus :size="18" />
        <span>New role</span>
      </PermissionButton>
      <button class="text-button secondary" type="button" :class="{ busy: loading }" :aria-busy="loading" :disabled="loading" @click="loadRoles">
        <RefreshCw :size="18" :class="{ spin: loading }" />
        <span>Refresh</span>
      </button>
    </div>

    <form v-if="showForm" class="inline-form role-form" @submit.prevent="saveRole">
      <label>
        <span>Name</span>
        <input v-model.trim="form.name" required placeholder="Auditor">
        <small v-if="fieldError('name')" class="field-error">{{ fieldError("name") }}</small>
      </label>
      <label>
        <span>Code</span>
        <input v-model.trim="form.code" required placeholder="auditor">
        <small v-if="fieldError('code')" class="field-error">{{ fieldError("code") }}</small>
      </label>
      <label class="wide-field">
        <span>Description</span>
        <input v-model.trim="form.description" placeholder="Read-only audit role">
      </label>

      <div class="permission-picker">
        <section v-for="group in groupedPermissions" :key="group.module" class="permission-group">
          <strong>{{ group.module }}</strong>
          <label v-for="permission in group.items" :key="permission.code" class="permission-option">
            <input
              type="checkbox"
              :checked="hasPermission(permission.code)"
              @change="togglePermission(permission.code, $event.target.checked)"
            >
            <span class="permission-meta">
              <span>{{ permission.name }}</span>
              <code>{{ permission.code }}</code>
            </span>
          </label>
        </section>
      </div>

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
            <th>Name</th>
            <th>Code</th>
            <th>Description</th>
            <th>Permissions</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="role in roles" :key="role.id">
            <td>{{ role.id }}</td>
            <td>{{ role.name }}</td>
            <td>{{ role.code }}</td>
            <td>{{ role.description || "-" }}</td>
            <td class="wrap-cell">
              {{ (role.permissions || []).join(", ") || "-" }}
            </td>
            <td>
              <div class="row-actions">
                <PermissionButton class="icon-button" :permission="permissions.systemRoleUpdate" title="Edit" @click="startEdit(role)">
                  <Pencil :size="17" />
                </PermissionButton>
                <PermissionButton class="icon-button danger" :permission="permissions.systemRoleDelete" title="Delete" @click="deleteRole(role)">
                  <Trash2 :size="17" />
                </PermissionButton>
              </div>
            </td>
          </tr>
          <tr v-if="!loading && roles.length === 0">
            <td colspan="6">
              <div class="empty-table">
                No roles
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>
