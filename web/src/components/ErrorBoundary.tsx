import { Component, type ReactNode } from "react";
import { bypassUnloadGuardOnce } from "@/lib/unloadGuard";

interface State {
  error: Error | null;
}

const AUTO_RELOAD_KEY = "npl_dom_autoreload_at";
const AUTO_RELOAD_WINDOW = 10_000; // ms：同一时间窗内只自动重载一次，避免无限刷新循环

/**
 * 判断是否为「React 记录的真实 DOM 被外部改动」类错误。典型来源：浏览器自动翻译、
 * CDN 注入脚本、浏览器扩展——它们移动/替换了 React 受控的节点，于是 React 下一次
 * insertBefore/removeChild 的参照节点已不是原父节点的孩子。
 */
function isDomMismatch(error: Error): boolean {
  const msg = String(error?.message || "");
  if (/insertBefore|removeChild|not a child|child of this node/i.test(msg)) return true;
  const name = (error as { name?: string }).name;
  return name === "NotFoundError" || name === "HierarchyRequestError";
}

/** Global error boundary so a render error shows a recovery screen, not a blank page. */
export class ErrorBoundary extends Component<{ children: ReactNode }, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidCatch(error: Error, info: unknown) {
    console.error("UI crashed:", error, info);
    // 自愈：这类崩溃多为外部改 DOM 造成的瞬时失配，整页重载后 React 会接管干净的
    // DOM 通常即恢复。用 sessionStorage 时间戳防循环：窗口内已自动重载过就不再重载，
    // 退回显示手动按钮（避免持续性错误把页面刷成无限循环）。
    if (isDomMismatch(error)) {
      let last = 0;
      try {
        last = Number(sessionStorage.getItem(AUTO_RELOAD_KEY) || 0);
      } catch {
        /* sessionStorage 不可用则跳过自愈，回退手动按钮 */
      }
      const now = Date.now();
      if (now - last > AUTO_RELOAD_WINDOW) {
        try {
          sessionStorage.setItem(AUTO_RELOAD_KEY, String(now));
        } catch {
          /* ignore */
        }
        // 让 SiteForm 的 beforeunload 守卫跳过这一次（崩溃时其卸载清理可能尚未执行，
        // 且 window.onbeforeunload=null 无法移除 addEventListener 注册的监听），避免
        // 自动重载被「未保存改动」弹窗挡住。
        bypassUnloadGuardOnce();
        location.reload();
      }
    }
  }

  render() {
    if (this.state.error) {
      return (
        <div className="flex min-h-screen flex-col items-center justify-center gap-4 p-6 text-center">
          <h1 className="text-xl font-semibold">页面出错了</h1>
          <p className="max-w-md text-sm text-muted-foreground">{this.state.error.message || "发生未知错误"}</p>
          <button
            className="rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground shadow-soft hover:bg-primary/90"
            onClick={() => location.reload()}
          >
            刷新页面
          </button>
        </div>
      );
    }
    return this.props.children;
  }
}
