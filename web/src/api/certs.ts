import { api } from "./client";
import type { Certificate } from "./types";

export const certsApi = {
  list: () => api.get<{ items: Certificate[] }>("/certs").then((r) => r.data.items),
  createManual: (name: string, certPem: string, keyPem: string) =>
    api.post("/certs/manual", { name, certPem, keyPem }),
  issueAcme: (name: string, domains: string[], email: string, env: string) =>
    api.post("/certs/acme", { name, domains, email, env }),
  update: (
    id: number,
    body: { name?: string; certPem?: string; keyPem?: string; domains?: string[]; email?: string; env?: string },
  ) => api.put(`/certs/${id}`, body),
  renew: (id: number) => api.post(`/certs/${id}/renew`),
  remove: (id: number) => api.delete(`/certs/${id}`),
};
