import { api } from "./client";
import type { AuditLog, Paged } from "./types";

export interface LogFilter {
  actor?: string;
  action?: string;
  from?: string;
  to?: string;
  page?: number;
  pageSize?: number;
}

export const logsApi = {
  list: (f: LogFilter) =>
    api
      .get<Paged<AuditLog>>("/logs", { params: f })
      .then((r) => r.data),
};

// Chinese labels for action codes (for the filter dropdown + display).
export const actionLabels: Record<string, string> = {
  "setup.init": "初始化管理员",
  "auth.login": "登录成功",
  "auth.login_fail": "登录失败",
  "auth.logout": "退出登录",
  "auth.totp_enroll": "启用两步验证",
  "user.create": "创建用户",
  "user.delete": "删除用户",
  "user.reset_password": "重置密码",
  "user.change_password": "修改本人密码",
  "site.create": "创建站点",
  "site.update": "修改站点",
  "site.delete": "删除站点",
  "site.toggle": "启停站点",
  "site.raw_edit": "编辑原始配置",
  "site.restore": "恢复配置备份",
  "ssl.manual": "上传手动证书",
  "ssl.acme_issue": "申请证书",
  "ssl.acme_renew": "续期证书",
  "ssl.disable": "关闭 SSL",
};

export function actionLabel(code: string): string {
  return actionLabels[code] ?? code;
}
