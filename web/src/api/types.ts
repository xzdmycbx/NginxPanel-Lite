export type Role = "admin" | "user";
export type SSLMode = "none" | "manual" | "acme";

export interface Me {
  needsSetup: boolean;
  authenticated: boolean;
  user?: {
    id: number;
    username: string;
    role: Role;
    totpEnabled: boolean;
  };
}

export interface ProxyLocation {
  path: string;
  upstreamTargets: string[];
  websocketUpgrade: boolean;
  cacheEnabled: boolean;
  extraConfig: string;
}

export interface Site {
  id: number;
  name: string;
  serverNames: string[];
  locations: ProxyLocation[] | null;
  upstreamTargets: string[] | null; // legacy fallback
  websocketUpgrade: boolean;
  forceHttpsRedirect: boolean;
  sslMode: SSLMode;
  certNotAfter?: string | null;
  lastRenewedAt?: string | null;
  acmeEmail?: string;
  acmeEnv?: string;
  renewError?: string;
  rawConfigOverride?: string;
  rawEdited?: boolean;
  enabled: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface SSLInfo {
  mode: SSLMode;
  domains: string[];
  notAfter?: string | null;
  daysLeft?: number | null;
  issuer?: string;
  env?: string;
  lastRenewedAt?: string | null;
  renewError?: string;
}

export interface UserView {
  id: number;
  username: string;
  role: Role;
  totpEnabled: boolean;
  createdAt: string;
}

export interface AuditLog {
  id: number;
  actorUsername: string;
  action: string;
  targetType: string;
  targetId: string;
  detail: string;
  result: string;
  ip: string;
  createdAt: string;
}

export interface Backup {
  timestamp: string;
  createdAt: string;
}

export interface Paged<T> {
  items: T[];
  total: number;
  page: number;
  pageSize: number;
}
