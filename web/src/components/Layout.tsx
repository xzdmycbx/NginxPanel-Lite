import { NavLink, Outlet, useNavigate } from "react-router-dom";
import { Globe, ScrollText, Users, KeyRound, LogOut, Leaf } from "lucide-react";
import { useAuth } from "@/auth/AuthProvider";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";

const nav = [
  { to: "/app/sites", label: "站点管理", icon: Globe, admin: false },
  { to: "/app/logs", label: "操作日志", icon: ScrollText, admin: false },
  { to: "/app/users", label: "用户管理", icon: Users, admin: true },
  { to: "/app/me", label: "我的账号", icon: KeyRound, admin: false },
];

export function Layout() {
  const { me, logout } = useAuth();
  const navigate = useNavigate();

  return (
    <div className="flex min-h-screen">
      <aside className="hidden w-64 shrink-0 flex-col border-r border-border/60 glass px-4 py-6 md:flex">
        <div className="mb-8 flex items-center gap-2.5 px-2">
          <div className="flex h-9 w-9 items-center justify-center rounded-xl bg-primary text-primary-foreground shadow-soft">
            <Leaf className="h-5 w-5" />
          </div>
          <div className="leading-tight">
            <div className="font-semibold">NginxPanel</div>
            <div className="text-xs text-muted-foreground">Lite 控制台</div>
          </div>
        </div>

        <nav className="flex flex-1 flex-col gap-1">
          {nav
            .filter((n) => !n.admin || me?.role === "admin")
            .map((n) => (
              <NavLink
                key={n.to}
                to={n.to}
                className={({ isActive }) =>
                  cn(
                    "flex items-center gap-3 rounded-lg px-3 py-2.5 text-sm font-medium transition-colors",
                    isActive
                      ? "bg-primary/12 text-primary"
                      : "text-muted-foreground hover:bg-accent hover:text-foreground",
                  )
                }
              >
                <n.icon className="h-[18px] w-[18px]" />
                {n.label}
              </NavLink>
            ))}
        </nav>

        <div className="mt-4 rounded-xl border border-border/60 bg-white/60 p-3">
          <div className="mb-2 px-1">
            <div className="truncate text-sm font-medium">{me?.username}</div>
            <div className="text-xs text-muted-foreground">
              {me?.role === "admin" ? "管理员" : "普通用户"}
            </div>
          </div>
          <Button variant="outline" size="sm" className="w-full" onClick={() => logout()}>
            <LogOut className="h-4 w-4" />
            退出登录
          </Button>
        </div>
      </aside>

      <div className="flex flex-1 flex-col">
        <header className="flex items-center justify-between border-b border-border/60 glass px-6 py-3 md:hidden">
          <div className="flex items-center gap-2 font-semibold">
            <Leaf className="h-5 w-5 text-primary" /> NginxPanel
          </div>
          <Button variant="ghost" size="sm" onClick={() => logout()}>
            退出
          </Button>
        </header>

        {/* mobile nav */}
        <div className="flex gap-1 overflow-x-auto border-b border-border/60 px-3 py-2 md:hidden">
          {nav
            .filter((n) => !n.admin || me?.role === "admin")
            .map((n) => (
              <button
                key={n.to}
                onClick={() => navigate(n.to)}
                className="whitespace-nowrap rounded-lg px-3 py-1.5 text-sm text-muted-foreground hover:bg-accent"
              >
                {n.label}
              </button>
            ))}
        </div>

        <main className="flex-1 p-6 lg:p-8">
          <div className="mx-auto max-w-6xl animate-fade-in">
            <Outlet />
          </div>
        </main>
      </div>
    </div>
  );
}
