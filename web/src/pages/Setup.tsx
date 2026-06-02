import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useNavigate } from "react-router-dom";
import { toast } from "sonner";
import { api, apiError } from "@/api/client";
import { AuthShell } from "@/components/AuthShell";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Spinner } from "@/components/ui/spinner";

const schema = z
  .object({
    username: z.string().min(3, "用户名至少 3 个字符").max(32, "用户名最多 32 个字符"),
    password: z.string().min(8, "密码至少 8 个字符").max(64, "密码最多 64 个字符"),
    confirm: z.string(),
  })
  .refine((d) => d.password === d.confirm, { path: ["confirm"], message: "两次输入的密码不一致" });

type Form = z.infer<typeof schema>;

export function Setup() {
  const navigate = useNavigate();
  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<Form>({ resolver: zodResolver(schema) });

  async function onSubmit(values: Form) {
    try {
      await api.post("/setup", { username: values.username, password: values.password });
      toast.success("管理员账号已创建，请绑定两步验证");
      navigate("/totp/enroll", { replace: true });
    } catch (e) {
      toast.error(apiError(e));
    }
  }

  return (
    <AuthShell title="初始化管理员" description="首次使用，请创建管理员账号">
      <form className="flex flex-col gap-4" onSubmit={handleSubmit(onSubmit)}>
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="username">用户名</Label>
          <Input id="username" autoComplete="username" placeholder="例如 admin" {...register("username")} />
          {errors.username && <p className="text-xs text-destructive">{errors.username.message}</p>}
        </div>
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="password">密码</Label>
          <Input id="password" type="password" autoComplete="new-password" {...register("password")} />
          {errors.password && <p className="text-xs text-destructive">{errors.password.message}</p>}
          <p className="text-xs text-muted-foreground">需包含大小写字母、数字、符号中的至少三类</p>
        </div>
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="confirm">确认密码</Label>
          <Input id="confirm" type="password" autoComplete="new-password" {...register("confirm")} />
          {errors.confirm && <p className="text-xs text-destructive">{errors.confirm.message}</p>}
        </div>
        <Button type="submit" className="mt-2 w-full" disabled={isSubmitting}>
          {isSubmitting && <Spinner />}
          创建并继续
        </Button>
      </form>
    </AuthShell>
  );
}
