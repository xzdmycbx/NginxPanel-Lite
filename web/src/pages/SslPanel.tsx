import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { ShieldCheck, AlertTriangle } from "lucide-react";
import { toast } from "sonner";
import { sitesApi } from "@/api/sites";
import { apiError, nginxOutput } from "@/api/client";
import type { Site, SSLMode } from "@/api/types";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Select } from "@/components/ui/select";
import { Segmented } from "@/components/ui/segmented";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Spinner } from "@/components/ui/spinner";

export function SslPanel({ site }: { site: Site }) {
  const qc = useQueryClient();
  const [mode, setMode] = useState<SSLMode>(site.sslMode);
  const [certPem, setCertPem] = useState("");
  const [keyPem, setKeyPem] = useState("");
  const [email, setEmail] = useState(site.acmeEmail ?? "");
  const [env, setEnv] = useState(site.acmeEnv || "staging");

  const { data: info } = useQuery({ queryKey: ["ssl", site.id], queryFn: () => sitesApi.ssl(site.id) });

  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ["ssl", site.id] });
    qc.invalidateQueries({ queryKey: ["site", String(site.id)] });
    qc.invalidateQueries({ queryKey: ["sites"] });
  };
  const onErr = (e: unknown) => toast.error(apiError(e), { description: nginxOutput(e) });

  const manual = useMutation({
    mutationFn: () => sitesApi.sslManual(site.id, certPem, keyPem),
    onSuccess: () => {
      toast.success("证书已保存并启用");
      setCertPem("");
      setKeyPem("");
      invalidate();
    },
    onError: onErr,
  });

  const acme = useMutation({
    mutationFn: () => sitesApi.sslAcme(site.id, email, env),
    onSuccess: () => {
      toast.success("证书申请成功");
      invalidate();
    },
    onError: onErr,
  });

  const renew = useMutation({
    mutationFn: () => sitesApi.sslRenew(site.id),
    onSuccess: () => {
      toast.success("证书已续期");
      invalidate();
    },
    onError: onErr,
  });

  const disable = useMutation({
    mutationFn: () => sitesApi.sslDisable(site.id),
    onSuccess: () => {
      toast.success("已关闭 SSL");
      setMode("none");
      invalidate();
    },
    onError: onErr,
  });

  const busy = manual.isPending || acme.isPending || renew.isPending || disable.isPending;

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <ShieldCheck className="h-5 w-5 text-primary" /> SSL 证书
        </CardTitle>
        <CardDescription>支持手动上传证书或通过 Let's Encrypt 自动签发并续期</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-5">
        {info && info.mode !== "none" && info.notAfter && (
          <div className="flex flex-wrap items-center gap-3 rounded-xl bg-muted/60 p-4">
            <Badge variant="success">
              {info.mode === "acme" ? "Let's Encrypt" : "手动证书"}
              {info.env ? `（${info.env}）` : ""}
            </Badge>
            <span className="text-sm text-muted-foreground">
              到期：{new Date(info.notAfter).toLocaleDateString("zh-CN")}
              {typeof info.daysLeft === "number" && `（剩 ${info.daysLeft} 天）`}
            </span>
            {info.mode === "acme" && (
              <Button size="sm" variant="outline" disabled={busy} onClick={() => renew.mutate()}>
                {renew.isPending && <Spinner />}
                立即续期
              </Button>
            )}
          </div>
        )}

        {info?.renewError && (
          <div className="flex items-start gap-2 rounded-xl bg-amber-50 p-3 text-sm text-amber-700">
            <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
            <span>上次续期失败：{info.renewError}</span>
          </div>
        )}

        <Segmented
          value={mode}
          onChange={setMode}
          options={[
            { label: "不启用", value: "none" },
            { label: "手动上传", value: "manual" },
            { label: "Let's Encrypt", value: "acme" },
          ]}
        />

        {mode === "manual" && (
          <div className="flex flex-col gap-3">
            <div className="flex flex-col gap-1.5">
              <Label>证书（fullchain.pem）</Label>
              <Textarea rows={5} placeholder="-----BEGIN CERTIFICATE-----" value={certPem} onChange={(e) => setCertPem(e.target.value)} />
            </div>
            <div className="flex flex-col gap-1.5">
              <Label>私钥（privkey.pem）</Label>
              <Textarea rows={5} placeholder="-----BEGIN PRIVATE KEY-----" value={keyPem} onChange={(e) => setKeyPem(e.target.value)} />
            </div>
            <Button className="self-start" disabled={busy || !certPem || !keyPem} onClick={() => manual.mutate()}>
              {manual.isPending && <Spinner />}
              保存并启用证书
            </Button>
          </div>
        )}

        {mode === "acme" && (
          <div className="flex flex-col gap-3">
            <div className="grid gap-3 sm:grid-cols-2">
              <div className="flex flex-col gap-1.5">
                <Label>申请邮箱</Label>
                <Input type="email" placeholder="admin@example.com" value={email} onChange={(e) => setEmail(e.target.value)} />
              </div>
              <div className="flex flex-col gap-1.5">
                <Label>环境</Label>
                <Select value={env} onChange={(e) => setEnv(e.target.value)}>
                  <option value="staging">测试环境（staging，推荐先测）</option>
                  <option value="production">正式环境（production）</option>
                </Select>
              </div>
            </div>
            <p className="text-xs text-muted-foreground">
              域名需已解析到本机且 80 端口可达。申请会自动验证并续期。
            </p>
            <Button className="self-start" disabled={busy || !email} onClick={() => acme.mutate()}>
              {acme.isPending ? (
                <>
                  <Spinner />
                  正在申请，可能需要 1 分钟…
                </>
              ) : (
                "申请并续期"
              )}
            </Button>
          </div>
        )}

        {mode === "none" && site.sslMode !== "none" && (
          <Button variant="outline" className="self-start" disabled={busy} onClick={() => disable.mutate()}>
            {disable.isPending && <Spinner />}
            关闭 SSL
          </Button>
        )}
      </CardContent>
    </Card>
  );
}
