import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Copy, Check } from "lucide-react";
import { toast } from "sonner";
import { api, apiError } from "@/api/client";
import { useAuth } from "@/auth/AuthProvider";
import { AuthShell } from "@/components/AuthShell";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Spinner } from "@/components/ui/spinner";

interface EnrollData {
  qrDataUri: string;
  secret: string;
  otpauthUrl: string;
}

export function TotpEnroll() {
  const navigate = useNavigate();
  const { refresh } = useAuth();
  const [data, setData] = useState<EnrollData | null>(null);
  const [code, setCode] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    api
      .post<EnrollData>("/auth/totp/enroll")
      .then((r) => setData(r.data))
      .catch((e) => toast.error(apiError(e, "无法生成两步验证")));
  }, []);

  async function activate() {
    setSubmitting(true);
    try {
      await api.post("/auth/totp/activate", { code });
      toast.success("两步验证已开启");
      refresh();
      navigate("/app/sites", { replace: true });
    } catch (e) {
      toast.error(apiError(e, "验证码错误"));
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <AuthShell title="绑定两步验证" description="使用 Authenticator 应用扫描二维码（必须开启）">
      <div className="flex flex-col items-center gap-5">
        {data ? (
          <img
            src={data.qrDataUri}
            alt="TOTP 二维码"
            className="h-44 w-44 rounded-xl border border-border bg-white p-2 shadow-soft"
          />
        ) : (
          <div className="flex h-44 w-44 items-center justify-center rounded-xl border border-border">
            <Spinner className="h-6 w-6 text-primary" />
          </div>
        )}

        {data && (
          <div className="w-full">
            <Label className="text-xs text-muted-foreground">无法扫描？手动输入密钥</Label>
            <div className="mt-1 flex items-center gap-2">
              <code className="flex-1 truncate rounded-lg bg-muted px-3 py-2 text-xs">{data.secret}</code>
              <Button
                type="button"
                variant="outline"
                size="icon"
                onClick={() => {
                  navigator.clipboard.writeText(data.secret);
                  setCopied(true);
                  setTimeout(() => setCopied(false), 1500);
                }}
              >
                {copied ? <Check className="h-4 w-4 text-primary" /> : <Copy className="h-4 w-4" />}
              </Button>
            </div>
          </div>
        )}

        <div className="w-full">
          <Label htmlFor="code">输入 6 位验证码</Label>
          <Input
            id="code"
            inputMode="numeric"
            autoComplete="one-time-code"
            maxLength={6}
            placeholder="000000"
            className="mt-1 text-center text-lg tracking-[0.4em]"
            value={code}
            onChange={(e) => setCode(e.target.value.replace(/\D/g, ""))}
            onKeyDown={(e) => e.key === "Enter" && code.length === 6 && activate()}
          />
        </div>

        <Button className="w-full" disabled={submitting || code.length !== 6} onClick={activate}>
          {submitting && <Spinner />}
          完成绑定
        </Button>
      </div>
    </AuthShell>
  );
}
