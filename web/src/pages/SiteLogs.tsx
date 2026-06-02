import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ScrollText, RefreshCw, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { siteLogsApi } from "@/api/sites";
import { apiError } from "@/api/client";
import type { Site } from "@/api/types";
import { useAuth } from "@/auth/AuthProvider";
import { Button } from "@/components/ui/button";
import { Segmented } from "@/components/ui/segmented";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Dialog, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from "@/components/ui/dialog";
import { Spinner } from "@/components/ui/spinner";

export function SiteLogs({ site }: { site: Site }) {
  const qc = useQueryClient();
  const { me } = useAuth();
  const [type, setType] = useState<"access" | "error">("access");
  const [confirmClear, setConfirmClear] = useState(false);

  const { data, isFetching, refetch } = useQuery({
    queryKey: ["site-logs", site.id, type],
    queryFn: () => siteLogsApi.tail(site.id, type),
    // 每次进入日志页签（组件重新挂载）或切换日志类型都强制拉取最新内容
    refetchOnMount: "always",
  });

  async function clear() {
    try {
      await siteLogsApi.clear(site.id, type);
      toast.success("已清空日志");
      qc.invalidateQueries({ queryKey: ["site-logs", site.id, type] });
    } catch (e) {
      toast.error(apiError(e));
    } finally {
      setConfirmClear(false);
    }
  }

  return (
    <Card>
      <CardHeader className="flex-row items-center justify-between">
        <div>
          <CardTitle className="flex items-center gap-2">
            <ScrollText className="h-5 w-5 text-primary" /> 访问 / 错误日志
          </CardTitle>
          <CardDescription>该域名独立的 nginx access / error 日志（最新 300 行）</CardDescription>
        </div>
        <div className="flex items-center gap-2">
          <Segmented
            value={type}
            onChange={setType}
            options={[
              { label: "访问日志", value: "access" },
              { label: "错误日志", value: "error" },
            ]}
          />
          <Button variant="outline" size="icon" title="刷新" onClick={() => refetch()}>
            <RefreshCw className={isFetching ? "h-4 w-4 animate-spin" : "h-4 w-4"} />
          </Button>
          {me?.role === "admin" && (
            <Button variant="outline" size="icon" title="清空" onClick={() => setConfirmClear(true)}>
              <Trash2 className="h-4 w-4 text-destructive" />
            </Button>
          )}
        </div>
      </CardHeader>
      <CardContent>
        <pre className="max-h-[50vh] overflow-auto rounded-lg bg-slate-900 p-4 text-xs leading-relaxed text-slate-100">
          {isFetching && !data ? (
            <Spinner className="h-5 w-5" />
          ) : data && data.length ? (
            data.join("\n")
          ) : (
            <span className="text-slate-400">暂无日志（该域名尚无流量，或日志尚未生成）</span>
          )}
        </pre>
      </CardContent>

      <Dialog open={confirmClear} onOpenChange={setConfirmClear}>
        <DialogHeader>
          <DialogTitle>清空日志</DialogTitle>
          <DialogDescription>
            确定清空该域名的「{type === "access" ? "访问" : "错误"}日志」吗？文件将被截断，已有内容不可恢复。
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="outline" onClick={() => setConfirmClear(false)}>
            取消
          </Button>
          <Button variant="destructive" onClick={clear}>
            确认清空
          </Button>
        </DialogFooter>
      </Dialog>
    </Card>
  );
}
