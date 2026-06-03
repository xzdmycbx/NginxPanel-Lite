import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { ShieldCheck } from "lucide-react";
import { toast } from "sonner";
import { sitesApi } from "@/api/sites";
import { certsApi } from "@/api/certs";
import { apiError, nginxOutput } from "@/api/client";
import type { Site } from "@/api/types";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Select, type SelectOption } from "@/components/ui/select";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Spinner } from "@/components/ui/spinner";

export function SiteSsl({ site }: { site: Site }) {
  const qc = useQueryClient();
  const { data: certs } = useQuery({ queryKey: ["certs"], queryFn: certsApi.list });
  const current = site.certId ? String(site.certId) : "";
  const [selected, setSelected] = useState<string>(current);

  const bind = useMutation({
    mutationFn: () => sitesApi.bindCert(site.id, selected ? Number(selected) : null),
    onSuccess: () => {
      toast.success(selected ? "已绑定证书" : "已关闭 SSL");
      qc.invalidateQueries({ queryKey: ["site", String(site.id)] });
      qc.invalidateQueries({ queryKey: ["sites"] });
    },
    onError: (e) => toast.error(apiError(e), { description: nginxOutput(e) }),
  });

  const options: SelectOption[] = [
    { value: "", label: "不启用 SSL" },
    ...(certs ?? []).map((cert) => ({ value: String(cert.id), label: `${cert.name}（${cert.domains.join(", ")}）` })),
  ];

  const changed = selected !== current;

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <ShieldCheck className="h-5 w-5 text-primary" /> SSL 证书
        </CardTitle>
        <CardDescription>
          证书统一在「SSL 证书」页上传 / 申请，这里为本站点选择要使用的证书并保存即可生效。
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {site.cert && (
          <div className="flex flex-wrap items-center gap-3 rounded-xl bg-muted/60 p-4">
            <Badge variant="success">{site.cert.source === "acme" ? "Let's Encrypt" : "手动证书"}</Badge>
            <span className="text-sm font-medium">{site.cert.name}</span>
            {site.cert.notAfter && (
              <span className="text-sm text-muted-foreground">
                到期：{new Date(site.cert.notAfter).toLocaleDateString("zh-CN")}
                {typeof site.cert.daysLeft === "number" && `（剩 ${site.cert.daysLeft} 天）`}
              </span>
            )}
          </div>
        )}

        <div className="flex flex-col gap-1.5">
          <Label>选择证书</Label>
          <Select searchable value={selected} onChange={setSelected} options={options} placeholder="不启用 SSL" />
          {certs && certs.length === 0 && (
            <p className="text-xs text-muted-foreground">
              还没有证书，先到{" "}
              <Link to="/app/ssl" className="text-primary underline-offset-2 hover:underline">
                SSL 证书
              </Link>{" "}
              页上传或申请。
            </p>
          )}
        </div>

        <Button className="self-start" disabled={bind.isPending || !changed || site.locked} onClick={() => bind.mutate()}>
          {bind.isPending && <Spinner />}
          保存 SSL 设置
        </Button>
        {site.locked && <p className="text-xs text-amber-700">站点已锁定，无法修改 SSL，请先由系统管理员解锁。</p>}
      </CardContent>
    </Card>
  );
}
