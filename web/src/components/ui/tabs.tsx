import type { ComponentType } from "react";
import { cn } from "@/lib/utils";

export interface TabItem<T extends string> {
  value: T;
  label: string;
  icon?: ComponentType<{ className?: string }>;
  /** 在标签文字后显示一个小红点（如「有未保存改动」提示） */
  dot?: boolean;
  /** 鼠标悬停在小红点上的提示文案 */
  dotTitle?: string;
}

interface TabsBarProps<T extends string> {
  value: T;
  onChange: (v: T) => void;
  items: TabItem<T>[];
  className?: string;
}

/**
 * 页面内分页夹 / 工具栏：横向可滚动的功能切换栏。
 * 仅切换组件状态，不改动路由。
 */
export function TabsBar<T extends string>({ value, onChange, items, className }: TabsBarProps<T>) {
  return (
    <div
      role="tablist"
      className={cn(
        "flex gap-1 overflow-x-auto rounded-2xl border border-border/70 bg-card p-1.5 shadow-card",
        className,
      )}
    >
      {items.map((it) => {
        const Icon = it.icon;
        const active = it.value === value;
        return (
          <button
            key={it.value}
            type="button"
            role="tab"
            aria-selected={active}
            onClick={() => onChange(it.value)}
            className={cn(
              "flex shrink-0 items-center gap-2 whitespace-nowrap rounded-xl px-4 py-2 text-sm font-medium transition-all active:scale-[0.98]",
              active
                ? "bg-primary/12 text-primary shadow-sm"
                : "text-muted-foreground hover:bg-accent hover:text-foreground",
            )}
          >
            {Icon && <Icon className="h-4 w-4" />}
            {it.label}
            {it.dot && (
              <span
                title={it.dotTitle}
                className="h-1.5 w-1.5 shrink-0 rounded-full bg-red-500"
              />
            )}
          </button>
        );
      })}
    </div>
  );
}
