import { api } from "./client";
import type { Role, UserView } from "./types";

export const usersApi = {
  list: () => api.get<{ items: UserView[] }>("/users").then((r) => r.data.items),
  create: (username: string, password: string, role: Role) => api.post("/users", { username, password, role }),
  resetPassword: (id: number, newPassword: string) => api.put(`/users/${id}/password`, { newPassword }),
  resetTotp: (id: number) => api.post(`/users/${id}/totp/reset`),
  disable: (id: number) => api.post(`/users/${id}/disable`),
  enable: (id: number) => api.post(`/users/${id}/enable`),
  remove: (id: number) => api.delete(`/users/${id}`),
  changeOwnPassword: (oldPassword: string, newPassword: string) =>
    api.put("/me/password", { oldPassword, newPassword }),
  selfTotpInit: (password: string) =>
    api
      .post<{ qrDataUri: string; secret: string; otpauthUrl: string }>("/me/totp/reset/init", { password })
      .then((r) => r.data),
  selfTotpConfirm: (code: string) => api.post("/me/totp/reset/confirm", { code }),
};
