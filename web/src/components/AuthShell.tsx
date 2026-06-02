import type { ReactNode } from "react";
import { Leaf } from "lucide-react";

export function AuthShell({
  title,
  description,
  children,
}: {
  title: string;
  description?: string;
  children: ReactNode;
}) {
  return (
    <div className="flex min-h-screen items-center justify-center p-4">
      <div className="w-full max-w-md animate-fade-in">
        <div className="mb-6 flex flex-col items-center gap-3 text-center">
          <div className="flex h-14 w-14 items-center justify-center rounded-2xl bg-primary text-primary-foreground shadow-card">
            <Leaf className="h-7 w-7" />
          </div>
          <div>
            <h1 className="text-xl font-semibold tracking-tight">{title}</h1>
            {description && <p className="mt-1 text-sm text-muted-foreground">{description}</p>}
          </div>
        </div>
        <div className="rounded-2xl border border-border/70 glass p-7 shadow-card">{children}</div>
        <p className="mt-6 text-center text-xs text-muted-foreground">NginxPanel-Lite · 反向代理可视化面板</p>
      </div>
    </div>
  );
}
