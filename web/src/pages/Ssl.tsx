import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Plus, Pencil, RefreshCw, Trash2, Upload } from "lucide-react";
import { toast } from "sonner";
import { certsApi } from "@/api/certs";
import { apiError, nginxOutput } from "@/api/client";
import type { Certificate } from "@/api/types";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Select } from "@/components/ui/select";
import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Dialog, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from "@/components/ui/dialog";
import { FullPageSpinner, Spinner } from "@/components/ui/spinner";

export function Ssl() {
  const qc = useQueryClient();
  const { data: certs, isLoading } = useQuery({ queryKey: ["certs"], queryFn: certsApi.list });
  const refetch = () => qc.invalidateQueries({ queryKey: ["certs"] });
  const onErr = (e: unknown) => toast.error(apiError(e), { description: nginxOutput(e) });

  const [showUpload, setShowUpload] = useState(false);
  const [showAcme, setShowAcme] = useState(false);
  const [toDelete, setToDelete] = useState<Certificate | null>(null);

  const [uName, setUName] = useState("");
  const [certPem, setCertPem] = useState("");
  const [keyPem, setKeyPem] = useState("");

  const [aName, setAName] = useState("");
  const [aDomains, setADomains] = useState("");
  const [aEmail, setAEmail] = useState("");
  const [aEnv, setAEnv] = useState("staging");

  const [editCert, setEditCert] = useState<Certificate | null>(null);
  const [eName, setEName] = useState("");
  const [eCertPem, setECertPem] = useState("");
  const [eKeyPem, setEKeyPem] = useState("");
  const [eDomains, setEDomains] = useState("");
  const [eEmail, setEEmail] = useState("");
  const [eEnv, setEEnv] = useState("staging");

  function openEdit(cert: Certificate) {
    setEditCert(cert);
    setEName(cert.name);
    setECertPem("");
    setEKeyPem("");
    setEDomains(cert.domains.join("\n"));
    setEEmail(cert.acmeEmail ?? "");
    setEEnv(cert.acmeEnv || "staging");
  }

  const upload = useMutation({
    mutationFn: () => certsApi.createManual(uName.trim(), certPem, keyPem),
    onSuccess: () => {
      toast.success("证书已上传");
      setShowUpload(false);
      setUName("");
      setCertPem("");
      setKeyPem("");
      refetch();
    },
    onError: onErr,
  });

  const issue = useMutation({
    mutationFn: () =>
      certsApi.issueAcme(
        aName.trim(),
        aDomains
          .split(/[\s,，]+/)
          .map((s) => s.trim())
          .filter(Boolean),
        aEmail.trim(),
        aEnv,
      ),
    onSuccess: () => {
      toast.success("证书申请成功");
      setShowAcme(false);
      setAName("");
      setADomains("");
      refetch();
    },
    onError: onErr,
  });

  const renew = useMutation({
    mutationFn: (id: number) => certsApi.renew(id),
    onSuccess: () => {
      toast.success("证书已续期");
      refetch();
    },
    onError: onErr,
  });

  const remove = useMutation({
    mutationFn: () => certsApi.remove(toDelete!.id),
    onSuccess: () => {
      toast.success("证书已删除");
      setToDelete(null);
      refetch();
    },
    onError: onErr,
  });

  const update = useMutation({
    mutationFn: () => {
      const cert = editCert!;
      return cert.source === "manual"
        ? certsApi.update(cert.id, { name: eName.trim(), certPem: eCertPem || undefined, keyPem: eKeyPem || undefined })
        : certsApi.update(cert.id, {
            name: eName.trim(),
            domains: eDomains
              .split(/[\s,，]+/)
              .map((s) => s.trim())
              .filter(Boolean),
            email: eEmail.trim(),
            env: eEnv,
          });
    },
    onSuccess: () => {
      toast.success("证书已修改");
      setEditCert(null);
      refetch();
    },
    onError: onErr,
  });

  if (isLoading) return <FullPageSpinner />;

  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">SSL 证书</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            全局管理证书：手动上传或 Let's Encrypt 申请，命名后到站点的「SSL 证书」页签选择使用
          </p>
        </div>
        <div className="flex gap-2">
          <Button variant="outline" onClick={() => setShowUpload(true)}>
            <Upload className="h-4 w-4" />
            上传证书
          </Button>
          <Button onClick={() => setShowAcme(true)}>
            <Plus className="h-4 w-4" />
            申请证书
          </Button>
        </div>
      </div>

      <Card>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>名称</TableHead>
              <TableHead className="whitespace-nowrap">类型</TableHead>
              <TableHead>域名</TableHead>
              <TableHead className="whitespace-nowrap">到期</TableHead>
              <TableHead>使用中</TableHead>
              <TableHead className="text-right">操作</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {certs && certs.length > 0 ? (
              certs.map((cert) => (
                <TableRow key={cert.id}>
                  <TableCell className="font-medium">{cert.name}</TableCell>
                  <TableCell className="whitespace-nowrap">
                    {cert.source === "acme" ? <Badge variant="success">Let's Encrypt</Badge> : <Badge>手动</Badge>}
                  </TableCell>
                  <TableCell className="font-mono text-xs text-muted-foreground">{cert.domains.join(", ")}</TableCell>
                  <TableCell className="whitespace-nowrap text-sm">
                    {cert.notAfter ? (
                      <span className={typeof cert.daysLeft === "number" && cert.daysLeft < 15 ? "text-destructive" : ""}>
                        {new Date(cert.notAfter).toLocaleDateString("zh-CN")}
                        {typeof cert.daysLeft === "number" && `（剩 ${cert.daysLeft} 天）`}
                      </span>
                    ) : cert.renewError ? (
                      <Badge variant="danger">签发失败</Badge>
                    ) : (
                      "-"
                    )}
                  </TableCell>
                  <TableCell className="text-xs text-muted-foreground">
                    {cert.inUseBy.length ? cert.inUseBy.join("、") : <span className="text-muted-foreground/60">未使用</span>}
                  </TableCell>
                  <TableCell>
                    <div className="flex justify-end gap-1">
                      <Button variant="ghost" size="icon" title="编辑" onClick={() => openEdit(cert)}>
                        <Pencil className="h-4 w-4" />
                      </Button>
                      {cert.source === "acme" && (
                        <Button
                          variant="ghost"
                          size="icon"
                          title="立即续期"
                          disabled={renew.isPending}
                          onClick={() => renew.mutate(cert.id)}
                        >
                          <RefreshCw className={renew.isPending ? "h-4 w-4 animate-spin" : "h-4 w-4"} />
                        </Button>
                      )}
                      <Button variant="ghost" size="icon" title="删除" onClick={() => setToDelete(cert)}>
                        <Trash2 className="h-4 w-4 text-destructive" />
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))
            ) : (
              <TableRow>
                <TableCell colSpan={6} className="py-12 text-center text-sm text-muted-foreground">
                  暂无证书，点击右上角「上传证书」或「申请证书」
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </Card>

      <Dialog open={showUpload} onOpenChange={setShowUpload} className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>上传证书</DialogTitle>
          <DialogDescription>粘贴 PEM 格式的证书与私钥，命名后即可在站点中选择</DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-4">
          <div className="flex flex-col gap-1.5">
            <Label>证书名称</Label>
            <Input value={uName} onChange={(e) => setUName(e.target.value)} placeholder="例如 example.com 通配符" />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label>证书（fullchain.pem）</Label>
            <Textarea rows={5} placeholder="-----BEGIN CERTIFICATE-----" value={certPem} onChange={(e) => setCertPem(e.target.value)} />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label>私钥（privkey.pem）</Label>
            <Textarea rows={5} placeholder="-----BEGIN PRIVATE KEY-----" value={keyPem} onChange={(e) => setKeyPem(e.target.value)} />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => setShowUpload(false)}>
            取消
          </Button>
          <Button disabled={upload.isPending || !uName || !certPem || !keyPem} onClick={() => upload.mutate()}>
            {upload.isPending && <Spinner />}
            保存
          </Button>
        </DialogFooter>
      </Dialog>

      <Dialog open={showAcme} onOpenChange={setShowAcme} className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>申请证书（Let's Encrypt）</DialogTitle>
          <DialogDescription>域名需已解析到本机且 80 端口可达。申请会自动验证并在到期前自动续期。</DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-4">
          <div className="flex flex-col gap-1.5">
            <Label>证书名称</Label>
            <Input value={aName} onChange={(e) => setAName(e.target.value)} placeholder="例如 app 证书" />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label>域名（一行一个或逗号分隔）</Label>
            <Textarea rows={3} placeholder={"app.example.com\nwww.example.com"} value={aDomains} onChange={(e) => setADomains(e.target.value)} />
          </div>
          <div className="grid gap-3 sm:grid-cols-2">
            <div className="flex flex-col gap-1.5">
              <Label>申请邮箱</Label>
              <Input type="email" placeholder="admin@example.com" value={aEmail} onChange={(e) => setAEmail(e.target.value)} />
            </div>
            <div className="flex flex-col gap-1.5">
              <Label>环境</Label>
              <Select
                value={aEnv}
                onChange={setAEnv}
                options={[
                  { value: "staging", label: "测试环境（staging，推荐先测）" },
                  { value: "production", label: "正式环境（production）" },
                ]}
              />
            </div>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => setShowAcme(false)}>
            取消
          </Button>
          <Button disabled={issue.isPending || !aName || !aDomains.trim() || !aEmail} onClick={() => issue.mutate()}>
            {issue.isPending ? (
              <>
                <Spinner />
                正在申请，可能需要 1 分钟…
              </>
            ) : (
              "申请并续期"
            )}
          </Button>
        </DialogFooter>
      </Dialog>

      <Dialog open={!!editCert} onOpenChange={(o) => !o && setEditCert(null)} className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>修改证书</DialogTitle>
          <DialogDescription>
            {editCert?.source === "acme"
              ? "修改名称，或更改域名 / 邮箱后将重新签发。保存后所有使用该证书的站点自动同步。"
              : "修改名称，或粘贴新的 PEM 替换证书内容（留空则只改名称）。替换后所有使用该证书的站点自动同步。"}
          </DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-4">
          <div className="flex flex-col gap-1.5">
            <Label>证书名称</Label>
            <Input value={eName} onChange={(e) => setEName(e.target.value)} />
          </div>
          {editCert?.source === "manual" ? (
            <>
              <div className="flex flex-col gap-1.5">
                <Label>证书（fullchain.pem，留空则不替换）</Label>
                <Textarea rows={4} placeholder="-----BEGIN CERTIFICATE-----" value={eCertPem} onChange={(e) => setECertPem(e.target.value)} />
              </div>
              <div className="flex flex-col gap-1.5">
                <Label>私钥（privkey.pem，留空则不替换）</Label>
                <Textarea rows={4} placeholder="-----BEGIN PRIVATE KEY-----" value={eKeyPem} onChange={(e) => setEKeyPem(e.target.value)} />
              </div>
            </>
          ) : (
            <>
              <div className="flex flex-col gap-1.5">
                <Label>域名（一行一个或逗号分隔，更改后会重新签发）</Label>
                <Textarea rows={3} value={eDomains} onChange={(e) => setEDomains(e.target.value)} />
              </div>
              <div className="grid gap-3 sm:grid-cols-2">
                <div className="flex flex-col gap-1.5">
                  <Label>申请邮箱</Label>
                  <Input type="email" value={eEmail} onChange={(e) => setEEmail(e.target.value)} />
                </div>
                <div className="flex flex-col gap-1.5">
                  <Label>环境</Label>
                  <Select
                    value={eEnv}
                    onChange={setEEnv}
                    options={[
                      { value: "staging", label: "测试环境（staging）" },
                      { value: "production", label: "正式环境（production）" },
                    ]}
                  />
                </div>
              </div>
            </>
          )}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => setEditCert(null)}>
            取消
          </Button>
          <Button disabled={update.isPending || !eName.trim()} onClick={() => update.mutate()}>
            {update.isPending ? (
              <>
                <Spinner />
                保存中…
              </>
            ) : (
              "保存"
            )}
          </Button>
        </DialogFooter>
      </Dialog>

      <Dialog open={!!toDelete} onOpenChange={(o) => !o && setToDelete(null)}>
        <DialogHeader>
          <DialogTitle>删除证书</DialogTitle>
          <DialogDescription>
            确定删除证书 <span className="font-medium text-foreground">{toDelete?.name}</span> 吗？若有站点正在使用将无法删除，请先在相关站点解绑。
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="outline" onClick={() => setToDelete(null)}>
            取消
          </Button>
          <Button variant="destructive" disabled={remove.isPending} onClick={() => remove.mutate()}>
            {remove.isPending && <Spinner />}
            确认删除
          </Button>
        </DialogFooter>
      </Dialog>
    </div>
  );
}
