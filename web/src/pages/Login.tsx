import { useForm } from "react-hook-form";
import { useNavigate } from "react-router-dom";
import { toast } from "sonner";
import { api, apiError } from "@/api/client";
import { useAuth } from "@/auth/AuthProvider";
import { AuthShell } from "@/components/AuthShell";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Spinner } from "@/components/ui/spinner";

interface Form {
  username: string;
  password: string;
}

export function Login() {
  const navigate = useNavigate();
  const { refresh } = useAuth();
  const {
    register,
    handleSubmit,
    formState: { isSubmitting },
  } = useForm<Form>();

  async function onSubmit(values: Form) {
    try {
      const { data } = await api.post<{ next: string }>("/auth/login", values);
      if (data.next === "needs_totp_enroll") navigate("/totp/enroll", { replace: true });
      else if (data.next === "needs_totp_code") navigate("/totp/verify", { replace: true });
      else {
        refresh();
        navigate("/app/sites", { replace: true });
      }
    } catch (e) {
      toast.error(apiError(e, "用户名或密码错误"));
    }
  }

  return (
    <AuthShell title="登录" description="欢迎回到 NginxPanel-Lite">
      <form className="flex flex-col gap-4" onSubmit={handleSubmit(onSubmit)}>
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="username">用户名</Label>
          <Input id="username" autoComplete="username" {...register("username", { required: true })} />
        </div>
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="password">密码</Label>
          <Input id="password" type="password" autoComplete="current-password" {...register("password", { required: true })} />
        </div>
        <Button type="submit" className="mt-2 w-full" disabled={isSubmitting}>
          {isSubmitting && <Spinner />}
          登录
        </Button>
      </form>
    </AuthShell>
  );
}
