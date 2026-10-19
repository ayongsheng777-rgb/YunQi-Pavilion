import { ref, computed } from "vue";
import { defineStore } from "pinia";

export interface LoginAttemptResult {
  success: boolean;
  needOtp: boolean;
  tmpToken: string;
}

export const useAuthStore = defineStore("auth", () => {
  const token = ref(localStorage.getItem("flat-nas-token") || "");
  const username = ref(localStorage.getItem("flat-nas-username") || "");
  const isLogged = computed(() => !!token.value);
  const password = ref("");

  /** 服务端会话状态：App 启动时调 GET /api/auth/me（OTP 记住设备靠 cookie），logged_in 为 true 则跳过登录框 */
  const serverSessionLoggedIn = ref(false);

  const checkServerSession = async () => {
    try {
      const res = await fetch("/api/auth/me");
      if (res.ok) {
        const data = await res.json();
        serverSessionLoggedIn.value = !!data?.logged_in;
      }
    } catch {
      // 网络异常时保持 false，不影响正常使用
    }
  };

  const getHeaders = (): Record<string, string> => {
    const headers: Record<string, string> = { "Content-Type": "application/json" };
    if (token.value) {
      headers["Authorization"] = `Bearer ${token.value}`;
    }
    return headers;
  };

  const login = async (usr: string, pwd: string): Promise<boolean> => {
    try {
      const res = await fetch("/api/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ username: usr, password: pwd }),
      });
      if (res.ok) {
        const data = await res.json();
        token.value = data.token;
        username.value = data.username;
        localStorage.setItem("flat-nas-token", data.token);
        localStorage.setItem("flat-nas-username", data.username);
        return true;
      }
      const data = await res.json();
      throw new Error(data.error || "Login failed");
    } catch (e: unknown) {
      console.error(e);
      throw e;
    }
  };

  const persistLogin = (tok: string, usr: string) => {
    token.value = tok;
    username.value = usr;
    localStorage.setItem("flat-nas-token", tok);
    localStorage.setItem("flat-nas-username", usr);
  };

  /**
   * 带 OTP 感知的登录：用户名密码正确后，
   * - 普通用户 → 直接登录成功
   * - 开了二次验证的用户 → 返回 { needOtp: true, tmpToken }，前端再弹验证码框
   */
  const loginDetailed = async (usr: string, pwd: string): Promise<LoginAttemptResult> => {
    try {
      const res = await fetch("/api/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ username: usr, password: pwd }),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) {
        throw new Error(data.error || "登录失败，请检查用户名和密码");
      }
      if (data.need_otp) {
        return { success: false, needOtp: true, tmpToken: String(data.tmp_token || "") };
      }
      persistLogin(data.token, data.username);
      return { success: true, needOtp: false, tmpToken: "" };
    } catch (e: unknown) {
      console.error(e);
      throw e;
    }
  };

  /** 第二步：提交 6 位验证码完成登录（服务端同时种 yq_device cookie 记住设备） */
  const verifyLoginOtp = async (
    tmpToken: string,
    code: string,
    rememberDevice: boolean,
  ): Promise<boolean> => {
    try {
      const res = await fetch("/api/login/otp", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ tmp_token: tmpToken, code, remember_device: rememberDevice }),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok || !data.success) {
        throw new Error(data.error || "验证码不对，再试一次");
      }
      persistLogin(data.token, data.username);
      return true;
    } catch (e: unknown) {
      console.error(e);
      throw e;
    }
  };

  const register = async (usr: string, pwd: string): Promise<boolean> => {    try {
      const res = await fetch("/api/register", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ username: usr, password: pwd }),
      });
      if (res.ok) return true;
      const data = await res.json();
      throw new Error(data.error || "Register failed");
    } catch (e: unknown) {
      console.error(e);
      throw e;
    }
  };

  const logout = () => {
    token.value = "";
    username.value = "";
    localStorage.removeItem("flat-nas-token");
    localStorage.removeItem("flat-nas-username");
  };

  const changePassword = (newPwd: string) => {
    password.value = newPwd;
  };

  const fetchUsers = async () => {
    try {
      const headers: Record<string, string> = {};
      if (token.value) headers["Authorization"] = `Bearer ${token.value}`;
      const res = await fetch("/api/admin/users", { headers });
      if (res.ok) {
        const data = await res.json();
        return data.users;
      }
      return [];
    } catch {
      return [];
    }
  };

  const addUser = async (usr: string, pwd: string): Promise<boolean> => {
    try {
      const headers: Record<string, string> = { "Content-Type": "application/json" };
      if (token.value) headers["Authorization"] = `Bearer ${token.value}`;
      const res = await fetch("/api/admin/users", {
        method: "POST",
        headers,
        body: JSON.stringify({ username: usr, password: pwd }),
      });
      if (res.ok) return true;
      const data = await res.json();
      throw new Error(data.error || "Add user failed");
    } catch (e) {
      throw e;
    }
  };

  const deleteUser = async (usr: string): Promise<boolean> => {
    try {
      const headers: Record<string, string> = {};
      if (token.value) headers["Authorization"] = `Bearer ${token.value}`;
      const res = await fetch(`/api/admin/users/${usr}`, { method: "DELETE", headers });
      if (res.ok) return true;
      throw new Error("Delete failed");
    } catch (e) {
      throw e;
    }
  };

  const uploadLicense = async (key: string): Promise<boolean> => {
    try {
      const headers: Record<string, string> = { "Content-Type": "application/json" };
      if (token.value) headers["Authorization"] = `Bearer ${token.value}`;
      const res = await fetch("/api/admin/license", {
        method: "POST",
        headers,
        body: JSON.stringify({ key }),
      });
      if (res.ok) return true;
      const data = await res.json();
      throw new Error(data.error || "Upload license failed");
    } catch (e) {
      throw e;
    }
  };

  return {
    token,
    username,
    isLogged,
    password,
    getHeaders,
    login,
    register,
    logout,
    changePassword,
    fetchUsers,
    addUser,
    deleteUser,
    uploadLicense,
    serverSessionLoggedIn,
    checkServerSession,
    loginDetailed,
    verifyLoginOtp,
  };
});
