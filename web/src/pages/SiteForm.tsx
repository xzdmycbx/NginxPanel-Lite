import { useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, Plus, X, FileCode, Trash2, Database } from "lucide-react";
import { toast } from "sonner";
import { sitesApi, type SiteInput } from "@/api/sites";
import { apiError, nginxOutput } from "@/api/client";
import type { Site } from "@/api/types";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Switch } from "@/components/ui/switch";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Dialog, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { FullPageSpinner, Spinner } from "@/components/ui/spinner";
import { useAuth } from "@/auth/AuthProvider";
import { SslPanel } from "./SslPanel";
import { SiteFiles } from "./SiteFiles";
import { SiteLogs } from "./SiteLogs";

interface LocState {
  path: string;
  upstreams: string[];
  websocketUpgrade: boolean;
  cacheEnabled: boolean;
  extraConfig: string;
}

const emptyLoc = (path = "/"): LocState => ({
  path,
  upstreams: [""],
  websocketUpgrade: false,
  cacheEnabled: false,
  extraConfig: "",
});

function locationsFromSite(site: Site): LocState[] {
  if (site.locations && site.locations.length) {
    return site.locations.map((l) => ({
      path: l.path,
      upstreams: l.upstreamTargets.length ? l.upstreamTargets : [""],
      websocketUpgrade: l.websocketUpgrade,
      cacheEnabled: l.cacheEnabled,
      extraConfig: l.extraConfig ?? "",
    }));
  }
  if (site.upstreamTargets && site.upstreamTargets.length) {
    return [{ ...emptyLoc("/"), upstreams: site.upstreamTargets, websocketUpgrade: site.websocketUpgrade }];
  }
  return [emptyLoc()];
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

  const { data: site, isLoading } = useQuery({
    queryKey: ["site", id],
    queryFn: () => sitesApi.get(id!),
    enabled: isEdit,
  });

  const [name, setName] = useState("");
  const [domains, setDomains] = useState<string[]>([""]);
  const [redirect, setRedirect] = useState(true);
  const [rawOverride, setRawOverride] = useState("");
  const [locations, setLocations] = useState<LocState[]>([emptyLoc()]);

  useEffect(() => {
    if (site) {
      setName(site.name);
      setDomains(site.serverNames.length ? site.serverNames : [""]);
      setRedirect(site.forceHttpsRedirect);
      setRawOverride(site.rawConfigOverride ?? "");
      setLocations(locationsFromSite(site));
    }
  }, [site]);

  // --- domain helpers ---
  const setDomain = (i: number, v: string) => setDomains((d) => d.map((x, idx) => (idx === i ? v : x)));
  const addDomain = () => setDomains((d) => [...d, ""]);
  const removeDomain = (i: number) => setDomains((d) => (d.length > 1 ? d.filter((_, idx) => idx !== i) : d));

  // --- location helpers ---
  const patchLoc = (i: number, patch: Partial<LocState>) =>
    setLocations((ls) => ls.map((l, idx) => (idx === i ? { ...l, ...patch } : l)));
  const addLoc = () => setLocations((ls) => [...ls, emptyLoc(`/path${ls.length}`)]);
  const removeLoc = (i: number) => setLocations((ls) => (ls.length > 1 ? ls.filter((_, idx) => idx !== i) : ls));
  const setUpstream = (li: number, ui: number, v: string) =>
    patchLoc(li, { upstreams: locations[li].upstreams.map((x, idx) => (idx === ui ? v : x)) });
  const addUpstream = (li: number) => patchLoc(li, { upstreams: [...locations[li].upstreams, ""] });
  const removeUpstream = (li: number, ui: number) =>
    patchLoc(li, {
      upstreams: locations[li].upstreams.length > 1 ? locations[li].upstreams.filter((_, idx) => idx !== ui) : locations[li].upstreams,
    });

  async function onSubmit() {
    const body: SiteInput = {
      name: name.trim(),
      serverNames: domains.map((d) => d.trim()).filter(Boolean),
      forceHttpsRedirect: redirect,
      rawConfigOverride: rawOverride,
      locations: locations.map((l) => ({
        path: l.path.trim() || "/",
        upstreamTargets: l.upstreams.map((u) => u.trim()).filter(Boolean),
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
        await sitesApi.update(id!, body);
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

  if (isEdit && isLoading) return <FullPageSpinner />;

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
              <div key={i} className="flex gap-2">
                <Input placeholder="例如 app.example.com" value={d} onChange={(e) => setDomain(i, e.target.value)} />
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
            <div key={li} className="rounded-xl border border-border/70 bg-muted/30 p-4">
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
                  <div key={ui} className="flex gap-2">
                    <Input
                      className="font-mono"
                      placeholder="http://127.0.0.1:3000 或 app:8080"
                      value={u}
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
            <Button onClick={onSubmit} disabled={submitting}>
              {submitting && <Spinner />}
              {isEdit ? "保存修改" : "创建站点"}
            </Button>
          </div>
        </CardContent>
      </Card>

      {isEdit && site && <SslPanel site={site} />}
      {isEdit && site && <SiteLogs site={site} />}
      {isEdit && site && isAdmin && <SiteFiles site={site} />}

      <Dialog open={!!preview} onOpenChange={(o) => !o && setPreview(null)} className="max-w-3xl">
        <DialogHeader>
          <DialogTitle>生成的 nginx 配置</DialogTitle>
        </DialogHeader>
        <pre className="max-h-[60vh] overflow-auto rounded-lg bg-muted p-4 text-xs leading-relaxed">{preview?.generated}</pre>
      </Dialog>
    </div>
  );
}
