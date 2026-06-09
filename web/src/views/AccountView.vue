<script setup>
import { reactive, ref, watch } from "vue";
import { KeyRound, Save } from "@lucide/vue";
import { validationErrorsByField } from "@/api/client";
import { useSessionStore } from "@/stores/session";

const session = useSessionStore();
const profileSaving = ref(false);
const passwordSaving = ref(false);
const profileError = ref("");
const passwordError = ref("");
const passwordFieldErrors = ref({});
const profileSuccess = ref("");
const passwordSuccess = ref("");

const profileForm = reactive({
  nickname: "",
  email: "",
  phone: "",
  avatar: "",
});

const passwordForm = reactive({
  current_password: "",
  new_password: "",
  confirm_password: "",
});

watch(
  () => session.user,
  (user) => {
    if (!user) return;
    profileForm.nickname = user.nickname || "";
    profileForm.email = user.email || "";
    profileForm.phone = user.phone || "";
    profileForm.avatar = user.avatar || "";
  },
  { immediate: true },
);

async function saveProfile() {
  profileSaving.value = true;
  profileError.value = "";
  profileSuccess.value = "";
  try {
    await session.updateProfile({
      nickname: profileForm.nickname,
      email: profileForm.email,
      phone: profileForm.phone,
      avatar: profileForm.avatar,
    });
    profileSuccess.value = "Saved";
  } catch (err) {
    profileError.value = err.message;
  } finally {
    profileSaving.value = false;
  }
}

async function savePassword() {
  clearPasswordErrors();
  if (passwordForm.new_password !== passwordForm.confirm_password) {
    passwordFieldErrors.value = {
      confirm_password: "Password confirmation does not match",
    };
    passwordError.value = "Password confirmation does not match";
    return;
  }

  passwordSaving.value = true;
  try {
    await session.changePassword({
      current_password: passwordForm.current_password,
      new_password: passwordForm.new_password,
    });
    passwordForm.current_password = "";
    passwordForm.new_password = "";
    passwordForm.confirm_password = "";
    passwordSuccess.value = "Saved";
  } catch (err) {
    passwordFieldErrors.value = validationErrorsByField(err);
    passwordError.value = err.message;
  } finally {
    passwordSaving.value = false;
  }
}

function clearPasswordErrors() {
  passwordError.value = "";
  passwordSuccess.value = "";
  passwordFieldErrors.value = {};
}

function passwordFieldError(field) {
  return passwordFieldErrors.value[field] || "";
}
</script>

<template>
  <section class="account-grid">
    <form class="inline-form account-form" @submit.prevent="saveProfile">
      <header class="form-header">
        <strong>Profile</strong>
        <span>{{ session.user?.username }}</span>
      </header>

      <label>
        <span>Nickname</span>
        <input v-model.trim="profileForm.nickname" placeholder="GOV2 Admin">
      </label>
      <label>
        <span>Email</span>
        <input v-model.trim="profileForm.email" type="email" placeholder="admin@gov2.local">
      </label>
      <label>
        <span>Phone</span>
        <input v-model.trim="profileForm.phone" placeholder="+1 555 0100">
      </label>
      <label>
        <span>Avatar</span>
        <input v-model.trim="profileForm.avatar" placeholder="https://example.com/avatar.png">
      </label>

      <div class="form-actions">
        <button class="text-button" type="submit" :class="{ busy: profileSaving }" :aria-busy="profileSaving" :disabled="profileSaving">
          <Save :size="18" />
          <span>Save</span>
        </button>
      </div>
      <p v-if="profileError" class="error">
        {{ profileError }}
      </p>
      <p v-if="profileSuccess" class="success">
        {{ profileSuccess }}
      </p>
    </form>

    <form class="inline-form account-form" @submit.prevent="savePassword">
      <header class="form-header">
        <strong>Password</strong>
        <KeyRound :size="18" />
      </header>

      <label>
        <span>Current password</span>
        <input v-model="passwordForm.current_password" required type="password" autocomplete="current-password">
        <small v-if="passwordFieldError('current_password')" class="field-error">
          {{ passwordFieldError("current_password") }}
        </small>
      </label>
      <label>
        <span>New password</span>
        <input
          v-model="passwordForm.new_password"
          required
          type="password"
          minlength="8"
          autocomplete="new-password"
        >
        <small v-if="passwordFieldError('new_password')" class="field-error">
          {{ passwordFieldError("new_password") }}
        </small>
      </label>
      <label>
        <span>Confirm password</span>
        <input
          v-model="passwordForm.confirm_password"
          required
          type="password"
          minlength="8"
          autocomplete="new-password"
        >
        <small v-if="passwordFieldError('confirm_password')" class="field-error">
          {{ passwordFieldError("confirm_password") }}
        </small>
      </label>

      <div class="form-actions">
        <button class="text-button" type="submit" :class="{ busy: passwordSaving }" :aria-busy="passwordSaving" :disabled="passwordSaving">
          <Save :size="18" />
          <span>Save</span>
        </button>
      </div>
      <p v-if="passwordError" class="error">
        {{ passwordError }}
      </p>
      <p v-if="passwordSuccess" class="success">
        {{ passwordSuccess }}
      </p>
    </form>
  </section>
</template>
