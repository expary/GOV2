<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { Pencil, Plus, RefreshCw, Save, Trash2, X } from "@lucide/vue";
import { validationErrorsByField } from "@/api/client";
import {
  deleteSystemSettingsId,
  getSystemSettings,
  postSystemSettings,
  putSystemSettingsId,
} from "@/api/requests";
import PermissionButton from "@/components/PermissionButton.vue";
import { permissions } from "@/permissions";
import { useSessionStore } from "@/stores/session";

const settings = ref([]);
const loading = ref(false);
const saving = ref(false);
const editingID = ref(null);
const error = ref("");
const fieldErrors = ref({});
const form = ref(emptyForm());
const session = useSessionStore();
let settingsRequestController = null;
let settingsRequestID = 0;

const showForm = computed(() => session.can(permissions.systemSettingCreate) || editingID.value !== null);
const canSave = computed(() => {
  const permission = editingID.value ? permissions.systemSettingUpdate : permissions.systemSettingCreate;
  return session.can(permission);
});

function emptyForm() {
  return {
    key: "",
    value_text: "{}",
    description: "",
  };
}

async function loadSettings() {
  settingsRequestController?.abort();
  const controller = new AbortController();
  settingsRequestController = controller;
  const requestID = ++settingsRequestID;
  loading.value = true;
  error.value = "";
  try {
    const result = await getSystemSettings({ signal: controller.signal });
    if (requestID !== settingsRequestID) return;
    settings.value = result;
  } catch (err) {
    if (err.name === "AbortError") return;
    if (requestID !== settingsRequestID) return;
    error.value = err.message;
  } finally {
    if (requestID === settingsRequestID) {
      loading.value = false;
      settingsRequestController = null;
    }
  }
}

function settingValueText(value) {
  if (value === undefined || value === null) {
    return "{}";
  }
  return JSON.stringify(value, null, 2);
}

function startCreate() {
  editingID.value = null;
  form.value = emptyForm();
  clearFormErrors();
}

function startEdit(setting) {
  editingID.value = setting.id;
  form.value = {
    key: setting.key,
    value_text: settingValueText(setting.value),
    description: setting.description || "",
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

function payload() {
  return {
    key: form.value.key,
    value: JSON.parse(form.value.value_text || "{}"),
    description: form.value.description,
  };
}

async function saveSetting() {
  saving.value = true;
  clearFormErrors();
  try {
    let body;
    try {
      body = payload();
    } catch {
      fieldErrors.value = { value: "Value must be valid JSON" };
      error.value = "invalid input";
      return;
    }
    if (editingID.value) {
      await putSystemSettingsId({
        params: { id: editingID.value },
        body,
      });
    } else {
      await postSystemSettings({
        body,
      });
    }
    cancelEdit();
    await loadSettings();
  } catch (err) {
    fieldErrors.value = validationErrorsByField(err);
    error.value = err.message;
  } finally {
    saving.value = false;
  }
}

async function deleteSetting(setting) {
  if (!window.confirm(`Delete setting "${setting.key}"?`)) return;
  loading.value = true;
  error.value = "";
  try {
    await deleteSystemSettingsId({ params: { id: setting.id } });
    if (editingID.value === setting.id) {
      cancelEdit();
    }
    await loadSettings();
  } catch (err) {
    error.value = err.message;
  } finally {
    loading.value = false;
  }
}

onMounted(loadSettings);

onBeforeUnmount(() => {
  settingsRequestController?.abort();
});
</script>

<template>
  <section class="panel">
    <div class="toolbar">
      <PermissionButton class="text-button" :permission="permissions.systemSettingCreate" @click="startCreate">
        <Plus :size="18" />
        <span>New setting</span>
      </PermissionButton>
      <button class="text-button secondary" type="button" :class="{ busy: loading }" :aria-busy="loading" :disabled="loading" @click="loadSettings">
        <RefreshCw :size="18" :class="{ spin: loading }" />
        <span>Refresh</span>
      </button>
    </div>

    <form v-if="showForm" class="inline-form setting-form" @submit.prevent="saveSetting">
      <label>
        <span>Key</span>
        <input v-model.trim="form.key" required placeholder="site.title">
        <small v-if="fieldError('key')" class="field-error">{{ fieldError("key") }}</small>
      </label>
      <label>
        <span>Description</span>
        <input v-model.trim="form.description" placeholder="Displayed application title">
      </label>
      <label class="json-field">
        <span>Value JSON</span>
        <textarea v-model="form.value_text" required spellcheck="false" />
        <small v-if="fieldError('value')" class="field-error">{{ fieldError("value") }}</small>
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
            <th>Key</th>
            <th>Value</th>
            <th>Description</th>
            <th>Updated</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="setting in settings" :key="setting.id">
            <td>{{ setting.id }}</td>
            <td>{{ setting.key }}</td>
            <td class="json-cell">
              <code>{{ settingValueText(setting.value) }}</code>
            </td>
            <td class="wrap-cell">
              {{ setting.description || "-" }}
            </td>
            <td>{{ new Date(setting.updated_at).toLocaleString() }}</td>
            <td>
              <div class="row-actions">
                <PermissionButton class="icon-button" :permission="permissions.systemSettingUpdate" title="Edit" @click="startEdit(setting)">
                  <Pencil :size="17" />
                </PermissionButton>
                <PermissionButton class="icon-button danger" :permission="permissions.systemSettingDelete" title="Delete" @click="deleteSetting(setting)">
                  <Trash2 :size="17" />
                </PermissionButton>
              </div>
            </td>
          </tr>
          <tr v-if="!loading && settings.length === 0">
            <td colspan="6">
              <div class="empty-table">
                No settings
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>
