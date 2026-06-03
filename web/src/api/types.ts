export type Role = "admin" | "user";
export type CertSource = "manual" | "acme";

export interface Me {
  needsSetup: boolean;
  authenticated: boolean;
  user?: {
    id: number;
    username: string;
    role: Role;
    systemAdmin: boolean;
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

export interface CertBrief {
  id: number;
  name: string;
  source: CertSource;
  notAfter?: string | null;
  daysLeft?: number | null;
}

export interface Site {
  id: number;
  name: string;
  serverNames: string[];
  locations: ProxyLocation[] | null;
  upstreamTargets: string[] | null; // legacy fallback
  websocketUpgrade: boolean;
  forceHttpsRedirect: boolean;
  certId?: number | null;
  cert?: CertBrief | null;
  rawConfigOverride?: string;
  rawEdited?: boolean;
  enabled: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface Certificate {
  id: number;
  name: string;
  source: CertSource;
  domains: string[];
  notAfter?: string | null;
  daysLeft?: number | null;
  issuer?: string;
  acmeEmail?: string;
  acmeEnv?: string;
  lastRenewedAt?: string | null;
  renewError?: string;
  inUseBy: string[];
  createdAt: string;
}

export interface UserView {
  id: number;
  username: string;
  role: Role;
  systemAdmin: boolean;
  disabled: boolean;
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
