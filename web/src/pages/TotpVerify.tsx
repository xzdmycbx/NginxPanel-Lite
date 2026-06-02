import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { toast } from "sonner";
import { api, apiError } from "@/api/client";
import { useAuth } from "@/auth/AuthProvider";
import { AuthShell } from "@/components/AuthShell";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Spinner } from "@/components/ui/spinner";

export function TotpVerify() {
  const navigate = useNavigate();
  const { refresh } = useAuth();
  const [code, setCode] = useState("");
  const [submitting, setSubmitting] = useState(false);

  async function verify() {
    setSubmitting(true);
    try {
      await api.post("/auth/totp/verify", { code });
      refresh();
      navigate("/app/sites", { replace: true });
    } catch (e) {
      toast.error(apiError(e, "验证码错误"));
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <AuthShell title="两步验证" description="请输入 Authenticator 应用中的 6 位验证码">
      <div className="flex flex-col gap-5">
        <div>
          <Label htmlFor="code">验证码</Label>
          <Input
            id="code"
            inputMode="numeric"
            autoComplete="one-time-code"
            maxLength={6}
            autoFocus
            placeholder="000000"
            className="mt-1 text-center text-lg tracking-[0.4em]"
            value={code}
            onChange={(e) => setCode(e.target.value.replace(/\D/g, ""))}
            onKeyDown={(e) => e.key === "Enter" && code.length === 6 && verify()}
          />
        </div>
        <Button className="w-full" disabled={submitting || code.length !== 6} onClick={verify}>
          {submitting && <Spinner />}
          验证并登录
        </Button>
      </div>
    </AuthShell>
  );
}
