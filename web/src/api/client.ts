import axios from "axios";

type Listener = () => void;

class Emitter {
  private map = new Map<string, Set<Listener>>();
  on(ev: string, fn: Listener) {
    const set = this.map.get(ev) ?? new Set();
    set.add(fn);
    this.map.set(ev, set);
    return () => set.delete(fn);
  }
  emit(ev: string) {
    this.map.get(ev)?.forEach((fn) => fn());
  }
}

/** authEvents bridges non-React axios interceptors to router navigation. */
export const authEvents = new Emitter();

export const api = axios.create({
  baseURL: "/api",
  withCredentials: true,
  headers: { "X-Requested-With": "XMLHttpRequest" },
});

api.interceptors.response.use(
  (res) => res,
  (error) => {
    const status = error.response?.status;
    const code = error.response?.data?.code as string | undefined;
    if (status === 409 && code === "needs_setup") authEvents.emit("needs_setup");
    else if (status === 403 && code === "needs_totp") authEvents.emit("needs_totp");
    else if (status === 401) authEvents.emit("unauthorized");
    return Promise.reject(error);
  },
);

/** apiError extracts the Chinese message from a failed request. */
export function apiError(e: unknown, fallback = "请求失败"): string {
  if (axios.isAxiosError(e)) {
    return (e.response?.data as { message?: string })?.message ?? fallback;
  }
  return fallback;
}

/** nginxOutput returns the nginx -t error detail when present. */
export function nginxOutput(e: unknown): string | undefined {
  if (axios.isAxiosError(e)) {
    return (e.response?.data as { nginxOutput?: string })?.nginxOutput;
  }
  return undefined;
}
