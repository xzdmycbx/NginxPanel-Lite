import { api } from "./client";
import type { Site, Backup, ProxyLocation } from "./types";

export interface SiteInput {
  name: string;
  serverNames: string[];
  locations: ProxyLocation[];
  forceHttpsRedirect: boolean;
  rawConfigOverride: string;
}

export interface SiteFileInfo {
  key: string;
  label: string;
}

export const siteFilesApi = {
  list: (id: number | string) =>
    api.get<{ items: SiteFileInfo[]; rawEdited: boolean }>(`/sites/${id}/files`).then((r) => r.data),
  get: (id: number | string, key: string) =>
    api.get<{ content: string }>(`/sites/${id}/file`, { params: { key } }).then((r) => r.data.content),
  save: (id: number | string, key: string, content: string) =>
    api.put(`/sites/${id}/file`, { content }, { params: { key } }),
};

export const siteLogsApi = {
  tail: (id: number | string, type: "access" | "error") =>
    api.get<{ lines: string[] }>(`/sites/${id}/logs`, { params: { type, lines: 300 } }).then((r) => r.data.lines),
  clear: (id: number | string, type: "access" | "error") =>
    api.post(`/sites/${id}/logs/clear`, null, { params: { type } }),
};

export const sitesApi = {
  list: () => api.get<{ items: Site[] }>("/sites").then((r) => r.data.items),
  get: (id: number | string) => api.get<Site>(`/sites/${id}`).then((r) => r.data),
  create: (body: SiteInput) => api.post<Site>("/sites", body).then((r) => r.data),
  update: (id: number | string, body: SiteInput) => api.put<Site>(`/sites/${id}`, body).then((r) => r.data),
  remove: (id: number | string) => api.delete(`/sites/${id}`),
  toggle: (id: number | string) => api.post(`/sites/${id}/toggle`),
  getConfig: (id: number | string) => api.get<{ content: string }>(`/sites/${id}/config`).then((r) => r.data.content),
  saveConfig: (id: number | string, content: string) => api.put(`/sites/${id}/config`, { content }),
  preview: (id: number | string) =>
    api.post<{ current: string; generated: string }>(`/sites/${id}/preview`).then((r) => r.data),
  bindCert: (id: number | string, certId: number | null) =>
    api.post<Site>(`/sites/${id}/cert`, { certId }).then((r) => r.data),
  lock: (id: number | string, code: string) => api.post<Site>(`/sites/${id}/lock`, { code }).then((r) => r.data),
  unlock: (id: number | string, code: string) => api.post<Site>(`/sites/${id}/unlock`, { code }).then((r) => r.data),
  backups: (id: number | string) => api.get<{ items: Backup[] }>(`/sites/${id}/backups`).then((r) => r.data.items),
  restore: (id: number | string, ts: string) => api.post(`/sites/${id}/backups/${ts}/restore`),
};
