<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { Pencil, Plus, RefreshCw, Save, Trash2, X } from "@lucide/vue";
import { validationErrorsByField } from "@/api/client";
import {
  deleteSystemDictionariesId,
  getSystemDictionaries,
  postSystemDictionaries,
  putSystemDictionariesId,
} from "@/api/requests";
import PermissionButton from "@/components/PermissionButton.vue";
import { permissions } from "@/permissions";
import { useSessionStore } from "@/stores/session";

const dictionaries = ref([]);
const loading = ref(false);
const saving = ref(false);
const editingID = ref(null);
const error = ref("");
const fieldErrors = ref({});
const form = ref(emptyForm());
const session = useSessionStore();
let dictionariesRequestController = null;
let dictionariesRequestID = 0;

const showForm = computed(() => session.can(permissions.systemDictionaryCreate) || editingID.value !== null);
const canSave = computed(() => {
  const permission = editingID.value ? permissions.systemDictionaryUpdate : permissions.systemDictionaryCreate;
  return session.can(permission);
});

function emptyForm() {
  return {
    code: "",
    name: "",
    items: [emptyItem()],
  };
}

function emptyItem() {
  return {
    label: "",
    value: "",
    sort: 0,
  };
}

async function loadDictionaries() {
  dictionariesRequestController?.abort();
  const controller = new AbortController();
  dictionariesRequestController = controller;
  const requestID = ++dictionariesRequestID;
  loading.value = true;
  error.value = "";
  try {
    const result = await getSystemDictionaries({ signal: controller.signal });
    if (requestID !== dictionariesRequestID) return;
    dictionaries.value = result;
  } catch (err) {
    if (err.name === "AbortError") return;
    if (requestID !== dictionariesRequestID) return;
    error.value = err.message;
  } finally {
    if (requestID === dictionariesRequestID) {
      loading.value = false;
      dictionariesRequestController = null;
    }
  }
}

function startCreate() {
  editingID.value = null;
  form.value = emptyForm();
  clearFormErrors();
}

function startEdit(dictionary) {
  editingID.value = dictionary.id;
  form.value = {
    code: dictionary.code,
    name: dictionary.name,
    items: (dictionary.items || []).map((item) => ({
      label: item.label,
      value: item.value,
      sort: item.sort || 0,
    })),
  };
  if (form.value.items.length === 0) {
    form.value.items = [emptyItem()];
  }
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

function addItem() {
  form.value.items.push(emptyItem());
}

function removeItem(index) {
  form.value.items.splice(index, 1);
  if (form.value.items.length === 0) {
    form.value.items.push(emptyItem());
  }
}

function dictionaryPayload() {
  return {
    code: form.value.code,
    name: form.value.name,
    items: form.value.items
      .map((item) => ({
        label: item.label.trim(),
        value: item.value.trim(),
        sort: Number(item.sort) || 0,
      }))
      .filter((item) => item.label || item.value),
  };
}

async function saveDictionary() {
  saving.value = true;
  clearFormErrors();
  try {
    const payload = dictionaryPayload();
    if (editingID.value) {
      await putSystemDictionariesId({
        params: { id: editingID.value },
        body: payload,
      });
    } else {
      await postSystemDictionaries({
        body: payload,
      });
    }
    cancelEdit();
    await loadDictionaries();
  } catch (err) {
    fieldErrors.value = validationErrorsByField(err);
    error.value = err.message;
  } finally {
    saving.value = false;
  }
}

async function deleteDictionary(dictionary) {
  if (!window.confirm(`Delete dictionary "${dictionary.name}"?`)) return;
  loading.value = true;
  error.value = "";
  try {
    await deleteSystemDictionariesId({ params: { id: dictionary.id } });
    if (editingID.value === dictionary.id) {
      cancelEdit();
    }
    await loadDictionaries();
  } catch (err) {
    error.value = err.message;
  } finally {
    loading.value = false;
  }
}

onMounted(loadDictionaries);

onBeforeUnmount(() => {
  dictionariesRequestController?.abort();
});
</script>

<template>
  <section class="panel">
    <div class="toolbar">
      <PermissionButton class="text-button" :permission="permissions.systemDictionaryCreate" @click="startCreate">
        <Plus :size="18" />
        <span>New dictionary</span>
      </PermissionButton>
      <button class="text-button secondary" type="button" :class="{ busy: loading }" :aria-busy="loading" :disabled="loading" @click="loadDictionaries">
        <RefreshCw :size="18" :class="{ spin: loading }" />
        <span>Refresh</span>
      </button>
    </div>

    <form v-if="showForm" class="inline-form dictionary-form" @submit.prevent="saveDictionary">
      <label>
        <span>Code</span>
        <input v-model.trim="form.code" required placeholder="ticket_priority">
        <small v-if="fieldError('code')" class="field-error">{{ fieldError("code") }}</small>
      </label>
      <label>
        <span>Name</span>
        <input v-model.trim="form.name" required placeholder="Ticket Priority">
        <small v-if="fieldError('name')" class="field-error">{{ fieldError("name") }}</small>
      </label>

      <div class="dictionary-item-editor">
        <header>
          <strong>Items</strong>
          <button class="text-button secondary" type="button" @click="addItem">
            <Plus :size="18" />
            <span>Add item</span>
          </button>
        </header>
        <small v-if="fieldError('items')" class="field-error">{{ fieldError("items") }}</small>
        <div v-for="(item, index) in form.items" :key="index" class="dictionary-item-row">
          <label>
            <span>Label</span>
            <input v-model.trim="item.label" required placeholder="High">
          </label>
          <label>
            <span>Value</span>
            <input v-model.trim="item.value" required placeholder="high">
          </label>
          <label>
            <span>Sort</span>
            <input v-model.number="item.sort" type="number" step="1">
          </label>
          <button class="icon-button danger" type="button" title="Remove item" @click="removeItem(index)">
            <Trash2 :size="17" />
          </button>
        </div>
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

    <div class="dictionary-grid">
      <article v-for="dictionary in dictionaries" :key="dictionary.id" class="dictionary-card">
        <header>
          <div>
            <strong>{{ dictionary.name }}</strong>
            <span>{{ dictionary.code }}</span>
          </div>
          <div class="row-actions">
            <span class="badge">{{ (dictionary.items || []).length }} items</span>
            <PermissionButton class="icon-button" :permission="permissions.systemDictionaryUpdate" title="Edit" @click="startEdit(dictionary)">
              <Pencil :size="17" />
            </PermissionButton>
            <PermissionButton class="icon-button danger" :permission="permissions.systemDictionaryDelete" title="Delete" @click="deleteDictionary(dictionary)">
              <Trash2 :size="17" />
            </PermissionButton>
          </div>
        </header>
        <ul>
          <li v-for="item in dictionary.items || []" :key="item.value">
            <span>{{ item.label }}</span>
            <code>{{ item.value }}</code>
          </li>
        </ul>
      </article>
    </div>
    <div v-if="!loading && dictionaries.length === 0" class="empty-state">
      <p>No dictionaries</p>
    </div>
  </section>
</template>
