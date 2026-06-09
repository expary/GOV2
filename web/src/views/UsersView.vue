<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import {
  ChevronLeft,
  ChevronRight,
  CircleCheck,
  CircleX,
  Pencil,
  Plus,
  RefreshCw,
  Search,
  Save,
  Trash2,
  X,
} from "@lucide/vue";
import { validationErrorsByField } from "@/api/client";
import {
  deleteSystemUsersId,
  getSystemDictionariesCodeCode,
  getSystemRoles,
  getSystemUsers,
  patchSystemUsersIdStatus,
  postSystemUsers,
  putSystemUsersId,
} from "@/api/requests";
import PermissionButton from "@/components/PermissionButton.vue";
import { permissions } from "@/permissions";
import { useSessionStore } from "@/stores/session";

const keyword = ref("");
const status = ref("");
const loading = ref(false);
const saving = ref(false);
const users = ref([]);
const roles = ref([]);
const userStatusDictionary = ref(null);
const total = ref(0);
const page = ref(1);
const pageSize = ref(20);
const editingID = ref(null);
const error = ref("");
const fieldErrors = ref({});
const form = ref(emptyForm());
const session = useSessionStore();
let usersRequestController = null;
let usersRequestID = 0;
let rolesRequestController = null;
let rolesRequestID = 0;
let statusDictionaryRequestController = null;
let statusDictionaryRequestID = 0;

const showForm = computed(() => session.can(permissions.systemUserCreate) || editingID.value !== null);
const canSave = computed(() => {
  const permission = editingID.value ? permissions.systemUserUpdate : permissions.systemUserCreate;
  return session.can(permission);
});
const roleNameByID = computed(() => {
  const out = new Map();
  roles.value.forEach((role) => out.set(role.id, role.name));
  return out;
});
const adminRoleID = computed(() => roles.value.find((role) => role.code === "admin")?.id || null);
const activeAdminCount = computed(() => users.value.filter((user) => isActiveAdmin(user)).length);
const fallbackStatusOptions = [
  { label: "Active", value: "active" },
  { label: "Disabled", value: "disabled" },
];
const validStatusValues = new Set(fallbackStatusOptions.map((item) => item.value));
const statusOptions = computed(() => {
  const items = userStatusDictionary.value?.items || [];
  if (items.length === 0) return fallbackStatusOptions;
  const options = [...items]
    .filter((item) => validStatusValues.has(item.value))
    .sort((a, b) => (a.sort || 0) - (b.sort || 0))
    .map((item) => ({
      label: item.label,
      value: item.value,
    }));
  return options.length > 0 ? options : fallbackStatusOptions;
});
const statusLabelByValue = computed(() => {
  const out = new Map();
  statusOptions.value.forEach((item) => out.set(item.value, item.label));
  return out;
});
const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)));
const pageStart = computed(() => (total.value === 0 ? 0 : (page.value - 1) * pageSize.value + 1));
const pageEnd = computed(() => Math.min(total.value, page.value * pageSize.value));

function emptyForm() {
  return {
    username: "",
    password: "",
    nickname: "",
    email: "",
    phone: "",
    avatar: "",
    role_ids: [],
    status: "active",
  };
}

async function loadUsers(nextPage = page.value) {
  usersRequestController?.abort();
  const controller = new AbortController();
  usersRequestController = controller;
  const requestID = ++usersRequestID;
  loading.value = true;
  error.value = "";
  page.value = nextPage;
  try {
    const result = await getSystemUsers({
      query: {
        page: page.value,
        page_size: pageSize.value,
        keyword: keyword.value.trim(),
        status: status.value,
      },
      signal: controller.signal,
    });
    if (requestID !== usersRequestID) return;
    users.value = result.items || [];
    total.value = result.total || 0;
    page.value = result.page || page.value;
    pageSize.value = result.page_size || pageSize.value;
  } catch (err) {
    if (err.name === "AbortError") return;
    if (requestID !== usersRequestID) return;
    error.value = err.message;
  } finally {
    if (requestID === usersRequestID) {
      loading.value = false;
      usersRequestController = null;
    }
  }
}

async function loadRoles() {
  rolesRequestController?.abort();
  const controller = new AbortController();
  rolesRequestController = controller;
  const requestID = ++rolesRequestID;
  try {
    const result = await getSystemRoles({ signal: controller.signal });
    if (requestID !== rolesRequestID) return;
    roles.value = result;
  } catch (err) {
    if (err.name === "AbortError") return;
    if (requestID !== rolesRequestID) return;
    throw err;
  } finally {
    if (requestID === rolesRequestID) {
      rolesRequestController = null;
    }
  }
}

async function loadUserStatusDictionary() {
  statusDictionaryRequestController?.abort();
  const controller = new AbortController();
  statusDictionaryRequestController = controller;
  const requestID = ++statusDictionaryRequestID;
  try {
    const result = await getSystemDictionariesCodeCode({
      params: { code: "user_status" },
      signal: controller.signal,
    });
    if (requestID !== statusDictionaryRequestID) return;
    userStatusDictionary.value = result;
  } catch (err) {
    if (err.name === "AbortError") return;
    if (requestID !== statusDictionaryRequestID) return;
    userStatusDictionary.value = null;
  } finally {
    if (requestID === statusDictionaryRequestID) {
      statusDictionaryRequestController = null;
    }
  }
}

function roleNames(roleIDs) {
  return (roleIDs || [])
    .map((id) => roleNameByID.value.get(id) || `#${id}`)
    .join(", ") || "-";
}

function isActiveAdmin(user) {
  return user.status === "active" && adminRoleID.value !== null && (user.role_ids || []).includes(adminRoleID.value);
}

function isLastActiveAdmin(user) {
  return isActiveAdmin(user) && activeAdminCount.value <= 1;
}

function protectsLastAdminRoleEdit() {
  if (!editingID.value || adminRoleID.value === null) return false;
  const original = users.value.find((user) => user.id === editingID.value);
  if (!original || !isLastActiveAdmin(original)) return false;
  return form.value.status !== "active" || !form.value.role_ids.includes(adminRoleID.value);
}

function protectsLastAdminRoleCheckbox(role) {
  if (!editingID.value || role.id !== adminRoleID.value) return false;
  const original = users.value.find((user) => user.id === editingID.value);
  return Boolean(original && isLastActiveAdmin(original) && form.value.role_ids.includes(adminRoleID.value));
}

function protectsLastAdminStatusEdit() {
  if (!editingID.value) return false;
  const original = users.value.find((user) => user.id === editingID.value);
  return Boolean(original && isLastActiveAdmin(original) && form.value.status === "active");
}

function statusLabel(value) {
  return statusLabelByValue.value.get(value) || value || "-";
}

function applyFilters() {
  loadUsers(1);
}

function resetFilters() {
  keyword.value = "";
  status.value = "";
  loadUsers(1);
}

function startCreate() {
  editingID.value = null;
  form.value = emptyForm();
  clearFormErrors();
}

function startEdit(user) {
  editingID.value = user.id;
  form.value = {
    username: user.username,
    password: "",
    nickname: user.nickname || "",
    email: user.email || "",
    phone: user.phone || "",
    avatar: user.avatar || "",
    role_ids: [...(user.role_ids || [])],
    status: user.status || "active",
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

function toggleRole(roleID, checked) {
  const current = new Set(form.value.role_ids);
  if (checked) {
    current.add(roleID);
  } else {
    current.delete(roleID);
  }
  form.value.role_ids = Array.from(current).sort((a, b) => a - b);
}

function hasRole(roleID) {
  return form.value.role_ids.includes(roleID);
}

async function saveUser() {
  if (protectsLastAdminRoleEdit()) {
    error.value = "Last active administrator cannot be removed";
    return;
  }
  saving.value = true;
  clearFormErrors();
  try {
    const payload = {
      ...form.value,
      role_ids: [...form.value.role_ids],
    };
    if (editingID.value && !payload.password) {
      delete payload.password;
    }
    if (editingID.value) {
      await putSystemUsersId({
        params: { id: editingID.value },
        body: payload,
      });
    } else {
      await postSystemUsers({
        body: payload,
      });
    }
    cancelEdit();
    await loadUsers();
  } catch (err) {
    fieldErrors.value = validationErrorsByField(err);
    error.value = err.message;
  } finally {
    saving.value = false;
  }
}

async function setStatus(user) {
  if (isLastActiveAdmin(user) && user.status === "active") return;
  const nextStatus = user.status === "active" ? "disabled" : "active";
  loading.value = true;
  error.value = "";
  try {
    await patchSystemUsersIdStatus({
      params: { id: user.id },
      body: { status: nextStatus },
    });
    await loadUsers();
  } catch (err) {
    error.value = err.message;
  } finally {
    loading.value = false;
  }
}

async function deleteUser(user) {
  if (isLastActiveAdmin(user)) return;
  if (!window.confirm(`Delete user "${user.username}"?`)) return;
  loading.value = true;
  error.value = "";
  try {
    await deleteSystemUsersId({ params: { id: user.id } });
    if (editingID.value === user.id) {
      cancelEdit();
    }
    await loadUsers();
  } catch (err) {
    error.value = err.message;
  } finally {
    loading.value = false;
  }
}

onMounted(async () => {
  await Promise.all([loadUsers(), loadRoles(), loadUserStatusDictionary()]);
});

onBeforeUnmount(() => {
  usersRequestController?.abort();
  rolesRequestController?.abort();
  statusDictionaryRequestController?.abort();
});
</script>

<template>
  <section class="panel">
    <form class="toolbar filter-toolbar" @submit.prevent="applyFilters">
      <label class="search-input">
        <Search :size="18" />
        <input v-model="keyword" placeholder="Search users">
      </label>
      <select v-model="status" class="filter-select">
        <option value="">
          All statuses
        </option>
        <option v-for="option in statusOptions" :key="option.value" :value="option.value">
          {{ option.label }}
        </option>
      </select>
      <select v-model="pageSize" class="filter-select" @change="applyFilters">
        <option :value="20">
          20 / page
        </option>
        <option :value="50">
          50 / page
        </option>
        <option :value="100">
          100 / page
        </option>
      </select>
      <button class="text-button" type="submit" :class="{ busy: loading }" :aria-busy="loading" :disabled="loading">
        <Search :size="18" />
        <span>Search</span>
      </button>
      <button class="text-button secondary" type="button" :class="{ busy: loading }" :aria-busy="loading" :disabled="loading" @click="resetFilters">
        <X :size="18" />
        <span>Reset</span>
      </button>
      <PermissionButton class="text-button" :permission="permissions.systemUserCreate" @click="startCreate">
        <Plus :size="18" />
        <span>New user</span>
      </PermissionButton>
      <button class="text-button secondary" type="button" :class="{ busy: loading }" :aria-busy="loading" :disabled="loading" @click="loadUsers()">
        <RefreshCw :size="18" :class="{ spin: loading }" />
        <span>Refresh</span>
      </button>
    </form>

    <form v-if="showForm" class="inline-form user-form" @submit.prevent="saveUser">
      <label>
        <span>Username</span>
        <input v-model.trim="form.username" required placeholder="jane">
        <small v-if="fieldError('username')" class="field-error">{{ fieldError("username") }}</small>
      </label>
      <label>
        <span>Password</span>
        <input
          v-model="form.password"
          :required="!editingID"
          type="password"
          minlength="8"
          autocomplete="new-password"
          :placeholder="editingID ? 'Leave blank to keep current' : 'Initial password'"
        >
        <small v-if="fieldError('password')" class="field-error">{{ fieldError("password") }}</small>
      </label>
      <label>
        <span>Nickname</span>
        <input v-model.trim="form.nickname" placeholder="Jane Doe">
      </label>
      <label>
        <span>Status</span>
        <select v-model="form.status" :disabled="protectsLastAdminStatusEdit()" :title="protectsLastAdminStatusEdit() ? 'Last active administrator must remain active' : undefined">
          <option v-for="option in statusOptions" :key="option.value" :value="option.value">
            {{ option.label }}
          </option>
        </select>
        <small v-if="fieldError('status')" class="field-error">{{ fieldError("status") }}</small>
      </label>
      <label>
        <span>Email</span>
        <input v-model.trim="form.email" type="email" placeholder="jane@gov2.local">
      </label>
      <label>
        <span>Phone</span>
        <input v-model.trim="form.phone" placeholder="+1 555 0100">
      </label>
      <label class="wide-field">
        <span>Avatar</span>
        <input v-model.trim="form.avatar" placeholder="https://example.com/avatar.png">
      </label>

      <div class="role-picker">
        <strong>Roles</strong>
        <small v-if="fieldError('role_ids')" class="field-error">{{ fieldError("role_ids") }}</small>
        <label v-for="role in roles" :key="role.id" class="permission-option">
          <input
            type="checkbox"
            :checked="hasRole(role.id)"
            :disabled="protectsLastAdminRoleCheckbox(role)"
            :title="protectsLastAdminRoleCheckbox(role) ? 'Last active administrator must keep the admin role' : undefined"
            @change="toggleRole(role.id, $event.target.checked)"
          >
          <span class="permission-meta">
            <span>{{ role.name }}</span>
            <code>{{ role.code }}</code>
          </span>
        </label>
      </div>

      <div class="form-actions">
        <button class="text-button" type="submit" :class="{ busy: saving }" :aria-busy="saving" :disabled="saving || !canSave || protectsLastAdminRoleEdit()">
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
            <th>Username</th>
            <th>Nickname</th>
            <th>Email</th>
            <th>Status</th>
            <th>Roles</th>
            <th>Last login</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="user in users" :key="user.id">
            <td>{{ user.id }}</td>
            <td>{{ user.username }}</td>
            <td>{{ user.nickname || "-" }}</td>
            <td>{{ user.email || "-" }}</td>
            <td>
              <span class="badge" :class="{ muted: user.status !== 'active' }">{{ statusLabel(user.status) }}</span>
            </td>
            <td class="wrap-cell">
              {{ roleNames(user.role_ids) }}
            </td>
            <td>{{ user.last_login_at ? new Date(user.last_login_at).toLocaleString() : "-" }}</td>
            <td>
              <div class="row-actions">
                <PermissionButton class="icon-button" :permission="permissions.systemUserUpdate" title="Edit" @click="startEdit(user)">
                  <Pencil :size="17" />
                </PermissionButton>
                <PermissionButton
                  class="icon-button"
                  :permission="permissions.systemUserUpdate"
                  :disabled="isLastActiveAdmin(user) && user.status === 'active'"
                  :title="isLastActiveAdmin(user) && user.status === 'active' ? 'Last active administrator cannot be disabled' : 'Toggle status'"
                  @click="setStatus(user)"
                >
                  <CircleX v-if="user.status === 'active'" :size="17" />
                  <CircleCheck v-else :size="17" />
                </PermissionButton>
                <PermissionButton
                  class="icon-button danger"
                  :permission="permissions.systemUserDelete"
                  :disabled="isLastActiveAdmin(user)"
                  :title="isLastActiveAdmin(user) ? 'Last active administrator cannot be deleted' : 'Delete'"
                  @click="deleteUser(user)"
                >
                  <Trash2 :size="17" />
                </PermissionButton>
              </div>
            </td>
          </tr>
          <tr v-if="!loading && users.length === 0">
            <td colspan="8">
              <div class="empty-table">
                No users
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <div class="table-footer">
      <span>{{ pageStart }}-{{ pageEnd }} of {{ total }}</span>
      <div class="row-actions">
        <button class="icon-button" type="button" title="Previous page" :disabled="loading || page <= 1" @click="loadUsers(page - 1)">
          <ChevronLeft :size="18" />
        </button>
        <span class="page-indicator">{{ page }} / {{ totalPages }}</span>
        <button class="icon-button" type="button" title="Next page" :disabled="loading || page >= totalPages" @click="loadUsers(page + 1)">
          <ChevronRight :size="18" />
        </button>
      </div>
    </div>
  </section>
</template>
