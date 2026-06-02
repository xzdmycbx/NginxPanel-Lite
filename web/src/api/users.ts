import { api } from "./client";
import type { Role, UserView } from "./types";

export const usersApi = {
  list: () => api.get<{ items: UserView[] }>("/users").then((r) => r.data.items),
  create: (username: string, password: string, role: Role) => api.post("/users", { username, password, role }),
  resetPassword: (id: number, newPassword: string) => api.put(`/users/${id}/password`, { newPassword }),
  resetTotp: (id: number) => api.post(`/users/${id}/totp/reset`),
  remove: (id: number) => api.delete(`/users/${id}`),
  changeOwnPassword: (oldPassword: string, newPassword: string) =>
    api.put("/me/password", { oldPassword, newPassword }),
};
