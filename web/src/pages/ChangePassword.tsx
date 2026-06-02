import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { toast } from "sonner";
import { usersApi } from "@/api/users";
import { apiError } from "@/api/client";
import { useAuth } from "@/auth/AuthProvider";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
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
  const { me } = useAuth();
  const {
    register,
    handleSubmit,
    reset,
    formState: { errors, isSubmitting },
  } = useForm<Form>({ resolver: zodResolver(schema) });

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
              {me?.role === "admin" ? <Badge>管理员</Badge> : <Badge variant="muted">普通用户</Badge>}
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
    </div>
  );
}
