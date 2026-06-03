import { useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, Plus, X, FileCode, Trash2, Database, Network, ShieldCheck, ScrollText, Lock } from "lucide-react";
import { toast } from "sonner";
import { sitesApi, type SiteInput } from "@/api/sites";
import { apiError, nginxOutput } from "@/api/client";
import type { Site } from "@/api/types";
import { isUnloadGuardBypassed } from "@/lib/unloadGuard";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Switch } from "@/components/ui/switch";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Dialog, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from "@/components/ui/dialog";
import { FullPageSpinner, Spinner } from "@/components/ui/spinner";
import { TabsBar, type TabItem } from "@/components/ui/tabs";
import { useAuth } from "@/auth/AuthProvider";
import { SiteSsl } from "./SiteSsl";
import { SiteFiles } from "./SiteFiles";
import { SiteLogs } from "./SiteLogs";

// 给每个可增删的输入行一个稳定 id 做 React key：数组下标当 key 在删中间行时会按位置
// 复用 DOM，破坏焦点 / 输入法 composition / 浏览器自动填充。用进程内自增计数器生成，
// 避免 crypto.randomUUID 在非安全上下文（纯 http 访问）下不可用。
let _rowSeq = 0;
const newRowId = () => `row-${++_rowSeq}`;

interface FieldRow {
  id: string;
  value: string;
}

const toRows = (values: string[]): FieldRow[] =>
  (values.length ? values : [""]).map((value) => ({ id: newRowId(), value }));

interface LocState {
  id: string;
  path: string;
  upstreams: FieldRow[];
  websocketUpgrade: boolean;
  cacheEnabled: boolean;
  extraConfig: string;
}

const emptyLoc = (path = "/"): LocState => ({
  id: newRowId(),
  path,
  upstreams: [{ id: newRowId(), value: "" }],
  websocketUpgrade: false,
  cacheEnabled: false,
  extraConfig: "",
});

type SiteTab = "config" | "ssl" | "logs" | "files";

function locationsFromSite(site: Site): LocState[] {
  if (site.locations && site.locations.length) {
    return site.locations.map((l) => ({
      id: newRowId(),
      path: l.path,
      upstreams: toRows(l.upstreamTargets),
      websocketUpgrade: l.websocketUpgrade,
      cacheEnabled: l.cacheEnabled,
      extraConfig: l.extraConfig ?? "",
    }));
  }
  if (site.upstreamTargets && site.upstreamTargets.length) {
    return [{ ...emptyLoc("/"), upstreams: toRows(site.upstreamTargets), websocketUpgrade: site.websocketUpgrade }];
  }
  return [emptyLoc()];
}

interface FormState {
  name: string;
  domains: FieldRow[];
  redirect: boolean;
  rawOverride: string;
  locations: LocState[];
}

// 表单初始值：useEffect 初始化与「未保存改动」基线都用它，保证两者完全一致
function initialFromSite(site: Site): FormState {
  return {
    name: site.name,
    domains: toRows(site.serverNames),
    redirect: site.forceHttpsRedirect,
    rawOverride: site.rawConfigOverride ?? "",
    locations: locationsFromSite(site),
  };
}

// 反向代理目标签名：每个 location 的「路径 + 目标列表」，用于判断保存是否改了代理去向
function proxyTargetsSig(locs: LocState[]): string {
  return JSON.stringify(locs.map((l) => [l.path.trim() || "/", l.upstreams.map((u) => u.value.trim()).filter(Boolean)]));
}

export function SiteForm() {
  const { id } = useParams();
  const isEdit = !!id;
  const navigate = useNavigate();
  const qc = useQueryClient();
  const { me } = useAuth();
  const isAdmin = me?.role === "admin";
  const [preview, setPreview] = useState<{ current: string; generated: string } | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [tab, setTab] = useState<SiteTab>("config");
  const [loadedId, setLoadedId] = useState<number | null>(null);
  const [baseline, setBaseline] = useState("");
  const [confirmSave, setConfirmSave] = useState(false);

  const { data: site, isLoading } = useQuery({
    queryKey: ["site", id],
    queryFn: () => sitesApi.get(id!),
    enabled: isEdit,
  });

  const [name, setName] = useState("");
  const [domains, setDomains] = useState<FieldRow[]>(() => [{ id: newRowId(), value: "" }]);
  const [redirect, setRedirect] = useState(true);
  const [rawOverride, setRawOverride] = useState("");
  const [locations, setLocations] = useState<LocState[]>(() => [emptyLoc()]);

  // 用服务端数据填充表单。仅在首次加载或切换到不同站点时回灌；同一站点的后台
  // 重新拉取（断网重连 / 缓存失效）不得覆盖未保存草稿，故按 site.id 设门、保存后再重置基线。
  function applyServerState(s: Site) {
    const init = initialFromSite(s);
    setName(init.name);
    setDomains(init.domains);
    setRedirect(init.redirect);
    setRawOverride(init.rawOverride);
    setLocations(init.locations);
    setBaseline(JSON.stringify(init));
    setLoadedId(s.id);
  }

  useEffect(() => {
    if (site && loadedId !== site.id) applyServerState(site);
  }, [site, loadedId]);

  const currentForm: FormState = { name, domains, redirect, rawOverride, locations };
  const dirty = isEdit && loadedId !== null && JSON.stringify(currentForm) !== baseline;
  const locked = !!site?.locked;

  // 有未保存改动时，拦截关闭标签页 / 刷新。
  useEffect(() => {
    if (!dirty) return;
    const onBeforeUnload = (e: BeforeUnloadEvent) => {
      if (isUnloadGuardBypassed()) return; // ErrorBoundary 自动重载时跳过提示
      e.preventDefault();
      e.returnValue = "";
    };
    window.addEventListener("beforeunload", onBeforeUnload);
    return () => window.removeEventListener("beforeunload", onBeforeUnload);
  }, [dirty]);

  // 反向代理目标是否相对加载时发生变化——用于保存前的二次确认（proxy_pass 改向会引流到新后端）。
  const targetsChanged =
    isEdit &&
    loadedId !== null &&
    baseline !== "" &&
    proxyTargetsSig(locations) !== proxyTargetsSig((JSON.parse(baseline) as FormState).locations);

  // --- domain helpers ---
  const setDomain = (i: number, v: string) => setDomains((d) => d.map((x, idx) => (idx === i ? { ...x, value: v } : x)));
  const addDomain = () => setDomains((d) => [...d, { id: newRowId(), value: "" }]);
  const removeDomain = (i: number) => setDomains((d) => (d.length > 1 ? d.filter((_, idx) => idx !== i) : d));

  // --- location helpers ---
  const patchLoc = (i: number, patch: Partial<LocState>) =>
    setLocations((ls) => ls.map((l, idx) => (idx === i ? { ...l, ...patch } : l)));
  const addLoc = () => setLocations((ls) => [...ls, emptyLoc(`/path${ls.length}`)]);
  const removeLoc = (i: number) => setLocations((ls) => (ls.length > 1 ? ls.filter((_, idx) => idx !== i) : ls));
  const setUpstream = (li: number, ui: number, v: string) =>
    patchLoc(li, { upstreams: locations[li].upstreams.map((x, idx) => (idx === ui ? { ...x, value: v } : x)) });
  const addUpstream = (li: number) => patchLoc(li, { upstreams: [...locations[li].upstreams, { id: newRowId(), value: "" }] });
  const removeUpstream = (li: number, ui: number) =>
    patchLoc(li, {
      upstreams: locations[li].upstreams.length > 1 ? locations[li].upstreams.filter((_, idx) => idx !== ui) : locations[li].upstreams,
    });

  async function onSubmit() {
    const body: SiteInput = {
      name: name.trim(),
      serverNames: domains.map((d) => d.value.trim()).filter(Boolean),
      forceHttpsRedirect: redirect,
      rawConfigOverride: rawOverride,
      locations: locations.map((l) => ({
        path: l.path.trim() || "/",
        upstreamTargets: l.upstreams.map((u) => u.value.trim()).filter(Boolean),
        websocketUpgrade: l.websocketUpgrade,
        cacheEnabled: l.cacheEnabled,
        extraConfig: l.extraConfig,
      })),
    };
    if (body.serverNames.length === 0) return toast.error("请至少填写一个域名");
    if (body.locations.some((l) => l.upstreamTargets.length === 0))
      return toast.error("每个反向代理路径都需要至少一个目标");

    setSubmitting(true);
    try {
      if (isEdit) {
        const saved = await sitesApi.update(id!, body);
        applyServerState(saved); // 以保存后的服务端值重置基线，红点 / 拦截随之清除
        toast.success("站点已更新");
        qc.invalidateQueries({ queryKey: ["sites"] });
        qc.invalidateQueries({ queryKey: ["site", id] });
      } else {
        const created = await sitesApi.create(body);
        toast.success("站点已创建");
        qc.invalidateQueries({ queryKey: ["sites"] });
        navigate(`/app/sites/${created.id}/edit`, { replace: true });
      }
    } catch (e) {
      toast.error(apiError(e), { description: nginxOutput(e) });
    } finally {
      setSubmitting(false);
    }
  }

  async function showPreview() {
    try {
      setPreview(await sitesApi.preview(id!));
    } catch (e) {
      toast.error(apiError(e));
    }
  }

  function handleSave() {
    // 改了反向代理去向属高影响操作，先二次确认。
    if (isEdit && targetsChanged) {
      setConfirmSave(true);
      return;
    }
    onSubmit();
  }

  if (isEdit && isLoading) return <FullPageSpinner />;

  const tabItems: TabItem<SiteTab>[] = [
    { value: "config", label: "代理配置", icon: Network, dot: dirty, dotTitle: "有未保存的配置改动" },
    { value: "ssl", label: "SSL 证书", icon: ShieldCheck },
    { value: "logs", label: "访问日志", icon: ScrollText },
    ...(isAdmin ? [{ value: "files" as const, label: "配置文件", icon: FileCode }] : []),
  ];

  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-center gap-3">
        <Button variant="ghost" size="icon" onClick={() => navigate("/app/sites")}>
          <ArrowLeft className="h-4 w-4" />
        </Button>
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">{isEdit ? "编辑站点" : "新建站点"}</h1>
          <p className="text-sm text-muted-foreground">配置域名绑定与反向代理路径</p>
        </div>
      </div>

      {isEdit && site && <TabsBar value={tab} onChange={setTab} items={tabItems} />}

      {locked && (
        <div className="flex items-center gap-2 rounded-xl border border-amber-300/60 bg-amber-50 px-4 py-3 text-sm text-amber-800">
          <Lock className="h-4 w-4 shrink-0" />
          此站点已被系统管理员锁定，无法修改配置 / SSL / 文件，请联系系统管理员解锁后再操作。
        </div>
      )}

      {/* 代理配置：基本信息 + 反向代理路径 + 高级，同属一个表单，统一保存 */}
      {(!isEdit || tab === "config") && (
        <>
      {/* 基本配置 */}
      <Card>
        <CardHeader>
          <CardTitle>基本配置</CardTitle>
          <CardDescription>域名写入 server_name</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-6">
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="name">站点名称</Label>
            <Input id="name" placeholder="留空则使用第一个域名" value={name} onChange={(e) => setName(e.target.value)} />
          </div>

          <div className="flex flex-col gap-2">
            <Label>域名绑定</Label>
            <p className="-mt-1 text-xs text-muted-foreground">一行一个域名，对应 nginx server_name</p>
            {domains.map((d, i) => (
              <div key={d.id} className="flex gap-2">
                <Input placeholder="例如 app.example.com" value={d.value} onChange={(e) => setDomain(i, e.target.value)} />
                <Button type="button" variant="ghost" size="icon" disabled={domains.length === 1} onClick={() => removeDomain(i)}>
                  <X className="h-4 w-4" />
                </Button>
              </div>
            ))}
            <Button type="button" variant="outline" size="sm" className="self-start" onClick={addDomain}>
              <Plus className="h-4 w-4" />
              添加域名
            </Button>
          </div>

          <div className="flex items-center gap-3">
            <Switch checked={redirect} onCheckedChange={setRedirect} id="redirect" />
            <Label htmlFor="redirect" className="cursor-pointer">启用 SSL 时强制跳转 HTTPS</Label>
          </div>
        </CardContent>
      </Card>

      {/* 反向代理路径 */}
      <Card>
        <CardHeader className="flex-row items-center justify-between">
          <div>
            <CardTitle>反向代理路径</CardTitle>
            <CardDescription>每个 location 可配置独立的目标、缓存与额外指令</CardDescription>
          </div>
          <Button type="button" variant="outline" size="sm" onClick={addLoc}>
            <Plus className="h-4 w-4" />
            添加路径
          </Button>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          {locations.map((loc, li) => (
            <div key={loc.id} className="rounded-xl border border-border/70 bg-muted/30 p-4">
              <div className="mb-3 flex items-center gap-2">
                <div className="flex-1">
                  <Label className="text-xs text-muted-foreground">location 路径</Label>
                  <Input
                    className="mt-1 font-mono"
                    placeholder="例如 / 或 /api 或 ~ \.php$"
                    value={loc.path}
                    onChange={(e) => patchLoc(li, { path: e.target.value })}
                  />
                </div>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="mt-5"
                  disabled={locations.length === 1}
                  title="删除该路径"
                  onClick={() => removeLoc(li)}
                >
                  <Trash2 className="h-4 w-4 text-destructive" />
                </Button>
              </div>

              <Label className="text-xs text-muted-foreground">反向代理目标（proxy_pass）</Label>
              <div className="mt-1 flex flex-col gap-2">
                {loc.upstreams.map((u, ui) => (
                  <div key={u.id} className="flex gap-2">
                    <Input
                      className="font-mono"
                      placeholder="http://127.0.0.1:3000 或 app:8080"
                      value={u.value}
                      onChange={(e) => setUpstream(li, ui, e.target.value)}
                    />
                    <Button type="button" variant="ghost" size="icon" disabled={loc.upstreams.length === 1} onClick={() => removeUpstream(li, ui)}>
                      <X className="h-4 w-4" />
                    </Button>
                  </div>
                ))}
                <Button type="button" variant="outline" size="sm" className="self-start" onClick={() => addUpstream(li)}>
                  <Plus className="h-4 w-4" />
                  添加目标（多个=负载均衡）
                </Button>
              </div>

              <div className="mt-4 flex flex-wrap gap-6">
                <div className="flex items-center gap-2.5">
                  <Switch checked={loc.websocketUpgrade} onCheckedChange={(v) => patchLoc(li, { websocketUpgrade: v })} />
                  <Label className="cursor-pointer">WebSocket</Label>
                </div>
                <div className="flex items-center gap-2.5">
                  <Switch checked={loc.cacheEnabled} onCheckedChange={(v) => patchLoc(li, { cacheEnabled: v })} />
                  <Label className="flex cursor-pointer items-center gap-1.5">
                    <Database className="h-3.5 w-3.5" /> 启用缓存
                  </Label>
                </div>
              </div>

              {isAdmin && (
                <div className="mt-4">
                  <Label className="text-xs text-muted-foreground">该路径的额外配置（仅管理员）</Label>
                  <Textarea
                    className="mt-1"
                    rows={2}
                    placeholder="# 追加到该 location 块的原始 nginx 指令，如 client_max_body_size 50m;"
                    value={loc.extraConfig}
                    onChange={(e) => patchLoc(li, { extraConfig: e.target.value })}
                  />
                </div>
              )}
            </div>
          ))}
        </CardContent>
      </Card>

      {/* 高级 */}
      <Card>
        <CardHeader>
          <CardTitle>高级</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          {isAdmin && (
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="raw">server 块额外原始配置（仅管理员）</Label>
              <Textarea id="raw" rows={3} placeholder="# 追加到 server 块的原始 nginx 指令" value={rawOverride} onChange={(e) => setRawOverride(e.target.value)} />
            </div>
          )}
          <div className="flex items-center justify-between border-t border-border/60 pt-5">
            {isEdit ? (
              <Button type="button" variant="outline" onClick={showPreview}>
                <FileCode className="h-4 w-4" />
                查看生成配置
              </Button>
            ) : (
              <span />
            )}
            <Button onClick={handleSave} disabled={submitting || locked}>
              {submitting && <Spinner />}
              {isEdit ? "保存修改" : "创建站点"}
            </Button>
          </div>
        </CardContent>
      </Card>
        </>
      )}

      {isEdit && site && tab === "ssl" && <SiteSsl site={site} />}
      {isEdit && site && tab === "logs" && <SiteLogs site={site} />}
      {isEdit && site && isAdmin && tab === "files" && <SiteFiles site={site} />}

      <Dialog open={!!preview} onOpenChange={(o) => !o && setPreview(null)} className="max-w-3xl">
        <DialogHeader>
          <DialogTitle>生成的 nginx 配置</DialogTitle>
        </DialogHeader>
        <pre className="max-h-[60vh] overflow-auto rounded-lg bg-muted p-4 text-xs leading-relaxed">{preview?.generated}</pre>
      </Dialog>

      <Dialog open={confirmSave} onOpenChange={setConfirmSave}>
        <DialogHeader>
          <DialogTitle>确认修改反向代理目标</DialogTitle>
          <DialogDescription>
            你修改了反向代理目标（proxy_pass）。保存后该站点的流量会被转发到新的后端地址，请确认目标无误。
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="outline" onClick={() => setConfirmSave(false)}>
            取消
          </Button>
          <Button
            disabled={submitting || locked}
            onClick={() => {
              setConfirmSave(false);
              onSubmit();
            }}
          >
            {submitting && <Spinner />}
            确认保存
          </Button>
        </DialogFooter>
      </Dialog>
    </div>
  );
}
