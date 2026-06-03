import { useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { ShieldCheck } from "lucide-react";
import { toast } from "sonner";
import { usersApi } from "@/api/users";
import { apiError } from "@/api/client";
import { useAuth } from "@/auth/AuthProvider";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Dialog, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from "@/components/ui/dialog";
import { Spinner } from "@/components/ui/spinner";

const schema = z
  .object({
    oldPassword: z.string().min(1, "请输入原密码"),
    newPassword: z.string().min(8, "新密码至少 8 个字符").max(64, "新密码最多 64 个字符"),
    confirm: z.string(),
  })
  .refine((d) => d.newPassword === d.confirm, { path: ["confirm"], message: "两次输入的密码不一致" });

type Form = z.infer<typeof schema>;

export function ChangePassword() {
  const { me, logout } = useAuth();
  const {
    register,
    handleSubmit,
    reset,
    formState: { errors, isSubmitting },
  } = useForm<Form>({ resolver: zodResolver(schema) });

  // 自助重置两步验证：输入当前密码 -> 扫新二维码并验证 -> 立即重绑 + 吊销所有会话
  const [totpOpen, setTotpOpen] = useState(false);
  const [totpPwd, setTotpPwd] = useState("");
  const [totpCode, setTotpCode] = useState("");
  const [enroll, setEnroll] = useState<{ qrDataUri: string; secret: string } | null>(null);
  const [totpBusy, setTotpBusy] = useState(false);

  function closeTotp() {
    setTotpOpen(false);
    setTotpPwd("");
    setTotpCode("");
    setEnroll(null);
  }
  async function totpInit() {
    setTotpBusy(true);
    try {
      setEnroll(await usersApi.selfTotpInit(totpPwd));
    } catch (e) {
      toast.error(apiError(e));
    } finally {
      setTotpBusy(false);
    }
  }
  async function totpConfirm() {
    setTotpBusy(true);
    try {
      await usersApi.selfTotpConfirm(totpCode);
      toast.success("两步验证已重新绑定，请用新验证器重新登录");
      closeTotp();
      await logout(); // 会话已吊销，回到登录页
    } catch (e) {
      toast.error(apiError(e));
    } finally {
      setTotpBusy(false);
    }
  }

  async function onSubmit(v: Form) {
    try {
      await usersApi.changeOwnPassword(v.oldPassword, v.newPassword);
      toast.success("密码修改成功");
      reset();
    } catch (e) {
      toast.error(apiError(e));
    }
  }

  return (
    <div className="flex flex-col gap-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">我的账号</h1>
        <p className="mt-1 text-sm text-muted-foreground">账号信息与密码管理</p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>账号信息</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-wrap gap-x-12 gap-y-3 text-sm">
          <div>
            <div className="text-muted-foreground">用户名（不可修改）</div>
            <div className="mt-0.5 font-medium">{me?.username}</div>
          </div>
          <div>
            <div className="text-muted-foreground">角色</div>
            <div className="mt-0.5">
              {me?.systemAdmin ? (
                <Badge>系统管理员</Badge>
              ) : me?.role === "admin" ? (
                <Badge variant="secondary">管理员</Badge>
              ) : (
                <Badge variant="muted">普通用户</Badge>
              )}
            </div>
          </div>
        </CardContent>
      </Card>

      <Card className="max-w-xl">
        <CardHeader>
          <CardTitle>修改密码</CardTitle>
          <CardDescription>修改后其他设备上的登录将失效</CardDescription>
        </CardHeader>
        <CardContent>
          <form className="flex flex-col gap-4" onSubmit={handleSubmit(onSubmit)}>
            <div className="flex flex-col gap-1.5">
              <Label>原密码</Label>
              <Input type="password" autoComplete="current-password" {...register("oldPassword")} />
              {errors.oldPassword && <p className="text-xs text-destructive">{errors.oldPassword.message}</p>}
            </div>
            <div className="flex flex-col gap-1.5">
              <Label>新密码</Label>
              <Input type="password" autoComplete="new-password" {...register("newPassword")} />
              {errors.newPassword && <p className="text-xs text-destructive">{errors.newPassword.message}</p>}
            </div>
            <div className="flex flex-col gap-1.5">
              <Label>确认新密码</Label>
              <Input type="password" autoComplete="new-password" {...register("confirm")} />
              {errors.confirm && <p className="text-xs text-destructive">{errors.confirm.message}</p>}
            </div>
            <Button type="submit" className="self-start" disabled={isSubmitting}>
              {isSubmitting && <Spinner />}
              保存新密码
            </Button>
          </form>
        </CardContent>
      </Card>

      <Card className="max-w-xl">
        <CardHeader>
          <CardTitle>两步验证（TOTP）</CardTitle>
          <CardDescription>
            重新绑定验证器：需输入当前密码，绑定成功后所有设备的登录都会立即失效，需用新验证器重新登录。
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Button variant="outline" onClick={() => setTotpOpen(true)}>
            <ShieldCheck className="h-4 w-4" />
            重置两步验证
          </Button>
        </CardContent>
      </Card>

      <Dialog open={totpOpen} onOpenChange={(o) => (o ? setTotpOpen(true) : closeTotp())}>
        <DialogHeader>
          <DialogTitle>重置两步验证</DialogTitle>
          <DialogDescription>
            {enroll
              ? "用 Authenticator 扫描下方新二维码，输入 6 位验证码完成绑定。"
              : "请先输入当前登录密码以验证身份。"}
          </DialogDescription>
        </DialogHeader>

        {!enroll ? (
          <div className="flex flex-col gap-1.5">
            <Label>当前密码</Label>
            <Input
              type="password"
              autoComplete="current-password"
              value={totpPwd}
              onChange={(e) => setTotpPwd(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && totpPwd && totpInit()}
            />
          </div>
        ) : (
          <div className="flex flex-col items-center gap-4">
            <img
              src={enroll.qrDataUri}
              alt="TOTP 二维码"
              className="h-44 w-44 rounded-xl border border-border bg-white p-2 shadow-soft"
            />
            <code className="w-full truncate rounded-lg bg-muted px-3 py-2 text-center text-xs">{enroll.secret}</code>
            <Input
              inputMode="numeric"
              autoComplete="one-time-code"
              maxLength={6}
              placeholder="000000"
              className="text-center text-lg tracking-[0.4em]"
              value={totpCode}
              onChange={(e) => setTotpCode(e.target.value.replace(/\D/g, ""))}
              onKeyDown={(e) => e.key === "Enter" && totpCode.length === 6 && totpConfirm()}
            />
          </div>
        )}

        <DialogFooter>
          <Button variant="outline" onClick={closeTotp}>
            取消
          </Button>
          {!enroll ? (
            <Button disabled={totpBusy || !totpPwd} onClick={totpInit}>
              {totpBusy && <Spinner />}
              下一步
            </Button>
          ) : (
            <Button disabled={totpBusy || totpCode.length !== 6} onClick={totpConfirm}>
              {totpBusy && <Spinner />}
              完成重新绑定
            </Button>
          )}
        </DialogFooter>
      </Dialog>
    </div>
  );
}
