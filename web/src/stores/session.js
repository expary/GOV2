import { defineStore } from "pinia";
import {
  getAuthProfile,
  postAuthLogin,
  postAuthLogout,
  putAuthPassword,
  putAuthProfile,
} from "@/api/requests";
import { permissions as permissionCodes } from "@/permissions";

const tokenKey = "gov2_token";

export const useSessionStore = defineStore("session", {
  state: () => ({
    token: localStorage.getItem(tokenKey) || "",
    profile: null,
  }),
  getters: {
    user: (state) => state.profile?.user || null,
    permissions: (state) => state.profile?.permissions || [],
    can: (state) => (permission) => {
      if (!permission) return true;
      const userPermissions = state.profile?.permissions || [];
      return userPermissions.includes(permissionCodes.all) || userPermissions.includes(permission);
    },
  },
  actions: {
    setToken(token) {
      this.token = token;
      localStorage.setItem(tokenKey, token);
    },
    clear() {
      this.token = "";
      this.profile = null;
      localStorage.removeItem(tokenKey);
    },
    async login(payload) {
      const result = await postAuthLogin({
        body: payload,
      });
      this.setToken(result.token);
      await this.loadProfile();
      return result;
    },
    async logout() {
      try {
        if (this.token) {
          await postAuthLogout();
        }
      } finally {
        this.clear();
      }
    },
    async loadProfile() {
      this.profile = await getAuthProfile();
      return this.profile;
    },
    async updateProfile(payload) {
      const user = await putAuthProfile({
        body: payload,
      });
      await this.loadProfile();
      return user;
    },
    async changePassword(payload) {
      return putAuthPassword({
        body: payload,
      });
    },
  },
});
