import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Plus, Pencil, Power, Trash2, ShieldCheck, Shield, Lock, LockOpen } from "lucide-react";
import { toast } from "sonner";
import { sitesApi } from "@/api/sites";
import { apiError, nginxOutput } from "@/api/client";
import type { Site } from "@/api/types";
import { useAuth } from "@/auth/AuthProvider";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Dialog, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from "@/components/ui/dialog";
import { FullPageSpinner, Spinner } from "@/components/ui/spinner";

function sslBadge(s: Site) {
  if (!s.cert) return <Badge variant="muted">未配置</Badge>;
  if (s.cert.source === "acme")
    return <Badge variant="success"><ShieldCheck className="mr-1 h-3 w-3" />{s.cert.name}</Badge>;
  return <Badge variant="default"><Shield className="mr-1 h-3 w-3" />{s.cert.name}</Badge>;
}

function targetSummary(s: Site): string {
  if (s.locations && s.locations.length) {
    return s.locations
      .map((l) => `${l.path}→${l.upstreamTargets[0] ?? "?"}${l.upstreamTargets.length > 1 ? ` (+${l.upstreamTargets.length - 1})` : ""}`)
      .join("　");
  }
  if (s.upstreamTargets && s.upstreamTargets.length) return `/→${s.upstreamTargets[0]}`;
  return "-";
}

export function Sites() {
  const navigate = useNavigate();
  const qc = useQueryClient();
  const { me } = useAuth();
  const [toDelete, setToDelete] = useState<Site | null>(null);
  const [toToggle, setToToggle] = useState<Site | null>(null);
  const [lockTarget, setLockTarget] = useState<{ site: Site; lock: boolean } | null>(null);
  const [lockCode, setLockCode] = useState("");

  const { data: sites, isLoading } = useQuery({ queryKey: ["sites"], queryFn: sitesApi.list });

  const handleErr = (e: unknown) => {
    const out = nginxOutput(e);
    toast.error(apiError(e), { description: out });
  };

  const toggle = useMutation({
    mutationFn: (id: number) => sitesApi.toggle(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["sites"] });
      toast.success("已更新站点状态");
    },
    onError: handleErr,
  });

  const remove = useMutation({
    mutationFn: (id: number) => sitesApi.remove(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["sites"] });
      toast.success("站点已删除");
      setToDelete(null);
    },
    onError: handleErr,
  });

  const lock = useMutation({
    mutationFn: () =>
      lockTarget!.lock
        ? sitesApi.lock(lockTarget!.site.id, lockCode)
        : sitesApi.unlock(lockTarget!.site.id, lockCode),
    onSuccess: () => {
      toast.success(lockTarget!.lock ? "站点已锁定" : "站点已解锁");
      setLockTarget(null);
      setLockCode("");
      qc.invalidateQueries({ queryKey: ["sites"] });
    },
    onError: handleErr,
  });

  if (isLoading) return <FullPageSpinner />;

  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">站点管理</h1>
          <p className="mt-1 text-sm text-muted-foreground">管理反向代理站点、域名绑定与 SSL 证书</p>
        </div>
        <Button onClick={() => navigate("/app/sites/new")}>
          <Plus className="h-4 w-4" />
          新建站点
        </Button>
      </div>

      <Card>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>名称 / 域名</TableHead>
              <TableHead>反向代理目标</TableHead>
              <TableHead>SSL</TableHead>
              <TableHead>状态</TableHead>
              <TableHead className="text-right">操作</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {sites && sites.length > 0 ? (
              sites.map((s) => (
                <TableRow key={s.id}>
                  <TableCell>
                    <div className="font-medium">{s.name}</div>
                    <div className="text-xs text-muted-foreground">{s.serverNames.join(", ")}</div>
                  </TableCell>
                  <TableCell className="font-mono text-xs text-muted-foreground">
                    {targetSummary(s)}
                  </TableCell>
                  <TableCell>{sslBadge(s)}</TableCell>
                  <TableCell className="whitespace-nowrap">
                    {s.enabled ? <Badge variant="success">运行中</Badge> : <Badge variant="muted">已停用</Badge>}
                    {s.locked && (
                      <Badge variant="warning" className="ml-1.5">
                        <Lock className="mr-1 h-3 w-3" />
                        已锁定
                      </Badge>
                    )}
                  </TableCell>
                  <TableCell>
                    <div className="flex justify-end gap-1">
                      <Button variant="ghost" size="icon" title="编辑" onClick={() => navigate(`/app/sites/${s.id}/edit`)}>
                        <Pencil className="h-4 w-4" />
                      </Button>
                      {me?.systemAdmin &&
                        (s.locked ? (
                          <Button variant="ghost" size="icon" title="解锁（需两步验证）" onClick={() => setLockTarget({ site: s, lock: false })}>
                            <LockOpen className="h-4 w-4 text-primary" />
                          </Button>
                        ) : (
                          <Button variant="ghost" size="icon" title="锁定（需两步验证）" onClick={() => setLockTarget({ site: s, lock: true })}>
                            <Lock className="h-4 w-4" />
                          </Button>
                        ))}
                      <Button
                        variant="ghost"
                        size="icon"
                        title={s.locked ? "已锁定" : s.enabled ? "停用" : "启用"}
                        disabled={s.locked}
                        onClick={() => (s.enabled ? setToToggle(s) : toggle.mutate(s.id))}
                      >
                        <Power className={s.enabled ? "h-4 w-4 text-primary" : "h-4 w-4 text-muted-foreground"} />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        title={s.locked ? "已锁定" : "删除"}
                        disabled={s.locked}
                        onClick={() => setToDelete(s)}
                      >
                        <Trash2 className="h-4 w-4 text-destructive" />
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))
            ) : (
              <TableRow>
                <TableCell colSpan={5} className="py-12 text-center text-sm text-muted-foreground">
                  暂无站点，点击右上角「新建站点」开始
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </Card>

      <Dialog open={!!toDelete} onOpenChange={(o) => !o && setToDelete(null)}>
        <DialogHeader>
          <DialogTitle>删除站点</DialogTitle>
          <DialogDescription>
            确定删除站点 <span className="font-medium text-foreground">{toDelete?.name}</span> 吗？此操作会移除其 nginx 配置，不可恢复。
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="outline" onClick={() => setToDelete(null)}>
            取消
          </Button>
          <Button variant="destructive" disabled={remove.isPending} onClick={() => toDelete && remove.mutate(toDelete.id)}>
            确认删除
          </Button>
        </DialogFooter>
      </Dialog>

      <Dialog open={!!toToggle} onOpenChange={(o) => !o && setToToggle(null)}>
        <DialogHeader>
          <DialogTitle>停用站点</DialogTitle>
          <DialogDescription>
            确定停用站点 <span className="font-medium text-foreground">{toToggle?.name}</span> 吗？停用后会移除其 nginx 配置，该域名将无法访问（可随时重新启用）。
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="outline" onClick={() => setToToggle(null)}>
            取消
          </Button>
          <Button
            variant="destructive"
            disabled={toggle.isPending}
            onClick={() => {
              if (toToggle) toggle.mutate(toToggle.id);
              setToToggle(null);
            }}
          >
            确认停用
          </Button>
        </DialogFooter>
      </Dialog>

      <Dialog open={!!lockTarget} onOpenChange={(o) => !o && (setLockTarget(null), setLockCode(""))}>
        <DialogHeader>
          <DialogTitle>{lockTarget?.lock ? "锁定站点" : "解锁站点"}</DialogTitle>
          <DialogDescription>
            {lockTarget?.lock ? (
              <>
                锁定后，<span className="font-medium text-foreground">任何人（包括你自己）</span>都无法修改站点{" "}
                <span className="font-medium text-foreground">{lockTarget?.site.name}</span> 的配置、SSL、文件或启停，直到解锁。请输入你的两步验证码确认。
              </>
            ) : (
              <>
                解锁站点 <span className="font-medium text-foreground">{lockTarget?.site.name}</span> 后将恢复编辑。请输入你的两步验证码确认。
              </>
            )}
          </DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-1.5">
          <Label>两步验证码</Label>
          <Input
            inputMode="numeric"
            autoComplete="one-time-code"
            maxLength={6}
            placeholder="000000"
            className="text-center text-lg tracking-[0.4em]"
            value={lockCode}
            onChange={(e) => setLockCode(e.target.value.replace(/\D/g, ""))}
            onKeyDown={(e) => e.key === "Enter" && lockCode.length === 6 && lock.mutate()}
          />
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => (setLockTarget(null), setLockCode(""))}>
            取消
          </Button>
          <Button
            variant={lockTarget?.lock ? "destructive" : "default"}
            disabled={lock.isPending || lockCode.length !== 6}
            onClick={() => lock.mutate()}
          >
            {lock.isPending && <Spinner />}
            {lockTarget?.lock ? "确认锁定" : "确认解锁"}
          </Button>
        </DialogFooter>
      </Dialog>
    </div>
  );
}
