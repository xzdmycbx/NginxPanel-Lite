import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { FileCode, AlertTriangle, Pencil } from "lucide-react";
import { toast } from "sonner";
import { siteFilesApi } from "@/api/sites";
import { apiError, nginxOutput } from "@/api/client";
import type { Site } from "@/api/types";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Dialog, DialogHeader, DialogTitle, DialogFooter } from "@/components/ui/dialog";
import { Spinner } from "@/components/ui/spinner";

export function SiteFiles({ site }: { site: Site }) {
  const qc = useQueryClient();
  const [editKey, setEditKey] = useState<string | null>(null);
  const [editLabel, setEditLabel] = useState("");
  const [content, setContent] = useState("");
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);

  const { data } = useQuery({ queryKey: ["site-files", site.id], queryFn: () => siteFilesApi.list(site.id) });

  async function open(key: string, label: string) {
    setEditKey(key);
    setEditLabel(label);
    setContent("");
    setLoading(true);
    try {
      setContent(await siteFilesApi.get(site.id, key));
    } catch (e) {
      toast.error(apiError(e));
    } finally {
      setLoading(false);
    }
  }

  async function save() {
    if (!editKey) return;
    setSaving(true);
    try {
      await siteFilesApi.save(site.id, editKey, content);
      toast.success("已保存并重载 nginx");
      setEditKey(null);
      qc.invalidateQueries({ queryKey: ["site-files", site.id] });
      qc.invalidateQueries({ queryKey: ["site", String(site.id)] });
    } catch (e) {
      toast.error(apiError(e), { description: nginxOutput(e) });
    } finally {
      setSaving(false);
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <FileCode className="h-5 w-5 text-primary" /> 配置文件（仅管理员）
        </CardTitle>
        <CardDescription>域名主文件与每个反向代理 location 文件均可单独编辑；保存前会 nginx -t 校验，失败自动回滚</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        {data?.rawEdited && (
          <div className="flex items-start gap-2 rounded-xl bg-amber-50 p-3 text-sm text-amber-700">
            <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
            <span>检测到手动编辑的配置文件。再次保存上方「结构化表单」会覆盖这些手动改动。</span>
          </div>
        )}
        <div className="divide-y divide-border/60 rounded-xl border border-border/60">
          {data?.items.map((f) => (
            <div key={f.key} className="flex items-center justify-between px-4 py-2.5">
              <div className="flex items-center gap-2 font-mono text-sm">
                {f.key === "site" ? <Badge variant="default">域名主文件</Badge> : <Badge variant="muted">location</Badge>}
                {f.label}
              </div>
              <Button variant="ghost" size="sm" onClick={() => open(f.key, f.label)}>
                <Pencil className="h-4 w-4" />
                编辑
              </Button>
            </div>
          ))}
        </div>
      </CardContent>

      <Dialog open={!!editKey} onOpenChange={(o) => !o && setEditKey(null)} className="max-w-3xl">
        <DialogHeader>
          <DialogTitle>编辑 {editLabel}</DialogTitle>
        </DialogHeader>
        {loading ? (
          <div className="flex h-40 items-center justify-center">
            <Spinner className="h-6 w-6 text-primary" />
          </div>
        ) : (
          <Textarea className="min-h-[50vh] text-xs leading-relaxed" spellCheck={false} value={content} onChange={(e) => setContent(e.target.value)} />
        )}
        <DialogFooter>
          <Button variant="outline" onClick={() => setEditKey(null)}>取消</Button>
          <Button disabled={saving || loading} onClick={save}>
            {saving && <Spinner />}
            保存并重载
          </Button>
        </DialogFooter>
      </Dialog>
    </Card>
  );
}
