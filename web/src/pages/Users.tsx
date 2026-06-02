import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { UserPlus, KeyRound, Trash2, ShieldOff } from "lucide-react";
import { toast } from "sonner";
import { usersApi } from "@/api/users";
import { apiError } from "@/api/client";
import type { Role, UserView } from "@/api/types";
import { useAuth } from "@/auth/AuthProvider";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select } from "@/components/ui/select";
import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Dialog, DialogHeader, DialogTitle, DialogFooter, DialogDescription } from "@/components/ui/dialog";
import { FullPageSpinner, Spinner } from "@/components/ui/spinner";

export function Users() {
  const qc = useQueryClient();
  const { me } = useAuth();
  const { data: users, isLoading } = useQuery({ queryKey: ["users"], queryFn: usersApi.list });

  const [showCreate, setShowCreate] = useState(false);
  const [resetUser, setResetUser] = useState<UserView | null>(null);
  const [totpUser, setTotpUser] = useState<UserView | null>(null);
  const [deleteUser, setDeleteUser] = useState<UserView | null>(null);

  // create form
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [role, setRole] = useState<Role>("user");
  const [newPassword, setNewPassword] = useState("");

  const refetch = () => qc.invalidateQueries({ queryKey: ["users"] });

  const create = useMutation({
    mutationFn: () => usersApi.create(username, password, role),
    onSuccess: () => {
      toast.success("用户已创建");
      setShowCreate(false);
      setUsername("");
      setPassword("");
      setRole("user");
      refetch();
    },
    onError: (e) => toast.error(apiError(e)),
  });

  const reset = useMutation({
    mutationFn: () => usersApi.resetPassword(resetUser!.id, newPassword),
    onSuccess: () => {
      toast.success("密码已重置");
      setResetUser(null);
      setNewPassword("");
    },
    onError: (e) => toast.error(apiError(e)),
  });

  const resetTotp = useMutation({
    mutationFn: () => usersApi.resetTotp(totpUser!.id),
    onSuccess: () => {
      toast.success("已重置两步验证，该用户下次登录需重新绑定");
      setTotpUser(null);
      refetch();
    },
    onError: (e) => toast.error(apiError(e)),
  });

  const remove = useMutation({
    mutationFn: () => usersApi.remove(deleteUser!.id),
    onSuccess: () => {
      toast.success("用户已删除");
      setDeleteUser(null);
      refetch();
    },
    onError: (e) => toast.error(apiError(e)),
  });

  if (isLoading) return <FullPageSpinner />;

  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">用户管理</h1>
          <p className="mt-1 text-sm text-muted-foreground">仅管理员可新增用户与重置密码（用户名不可修改）</p>
        </div>
        <Button onClick={() => setShowCreate(true)}>
          <UserPlus className="h-4 w-4" />
          新增用户
        </Button>
      </div>

      <Card>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>用户名</TableHead>
              <TableHead>角色</TableHead>
              <TableHead>两步验证</TableHead>
              <TableHead>创建时间</TableHead>
              <TableHead className="text-right">操作</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {users?.map((u) => (
              <TableRow key={u.id}>
                <TableCell className="font-medium">
                  {u.username}
                  {u.id === me?.id && <Badge variant="secondary" className="ml-2">本人</Badge>}
                </TableCell>
                <TableCell>
                  {u.role === "admin" ? <Badge>管理员</Badge> : <Badge variant="muted">普通用户</Badge>}
                </TableCell>
                <TableCell>
                  {u.totpEnabled ? <Badge variant="success">已开启</Badge> : <Badge variant="warning">未开启</Badge>}
                </TableCell>
                <TableCell className="text-sm text-muted-foreground">{u.createdAt}</TableCell>
                <TableCell>
                  <div className="flex justify-end gap-1">
                    <Button variant="ghost" size="icon" title="重置密码" onClick={() => setResetUser(u)}>
                      <KeyRound className="h-4 w-4" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      title="重置两步验证"
                      disabled={!u.totpEnabled}
                      onClick={() => setTotpUser(u)}
                    >
                      <ShieldOff className="h-4 w-4" />
                    </Button>
                    <Button variant="ghost" size="icon" title="删除" onClick={() => setDeleteUser(u)}>
                      <Trash2 className="h-4 w-4 text-destructive" />
                    </Button>
                  </div>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </Card>

      {/* create user */}
      <Dialog open={showCreate} onOpenChange={setShowCreate}>
        <DialogHeader>
          <DialogTitle>新增用户</DialogTitle>
          <DialogDescription>设置用户名、初始密码与角色</DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-4">
          <div className="flex flex-col gap-1.5">
            <Label>用户名</Label>
            <Input value={username} onChange={(e) => setUsername(e.target.value)} placeholder="3-32 个字符" />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label>初始密码</Label>
            <Input type="password" value={password} onChange={(e) => setPassword(e.target.value)} />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label>角色</Label>
            <Select value={role} onChange={(e) => setRole(e.target.value as Role)}>
              <option value="user">普通用户</option>
              <option value="admin">管理员</option>
            </Select>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => setShowCreate(false)}>取消</Button>
          <Button disabled={create.isPending || !username || !password} onClick={() => create.mutate()}>
            {create.isPending && <Spinner />}
            创建
          </Button>
        </DialogFooter>
      </Dialog>

      {/* reset password */}
      <Dialog open={!!resetUser} onOpenChange={(o) => !o && setResetUser(null)}>
        <DialogHeader>
          <DialogTitle>重置密码</DialogTitle>
          <DialogDescription>为用户 {resetUser?.username} 设置新密码，其现有会话将失效</DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-1.5">
          <Label>新密码</Label>
          <Input type="password" value={newPassword} onChange={(e) => setNewPassword(e.target.value)} />
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => setResetUser(null)}>取消</Button>
          <Button disabled={reset.isPending || !newPassword} onClick={() => reset.mutate()}>
            {reset.isPending && <Spinner />}
            确认重置
          </Button>
        </DialogFooter>
      </Dialog>

      {/* reset TOTP */}
      <Dialog open={!!totpUser} onOpenChange={(o) => !o && setTotpUser(null)}>
        <DialogHeader>
          <DialogTitle>重置两步验证</DialogTitle>
          <DialogDescription>
            将清除用户 {totpUser?.username} 的两步验证绑定，其会话将失效，下次登录需重新扫码绑定。用于该用户丢失验证器时的恢复。
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="outline" onClick={() => setTotpUser(null)}>取消</Button>
          <Button variant="destructive" disabled={resetTotp.isPending} onClick={() => resetTotp.mutate()}>
            {resetTotp.isPending && <Spinner />}
            确认重置
          </Button>
        </DialogFooter>
      </Dialog>

      {/* delete */}
      <Dialog open={!!deleteUser} onOpenChange={(o) => !o && setDeleteUser(null)}>
        <DialogHeader>
          <DialogTitle>删除用户</DialogTitle>
          <DialogDescription>确定删除用户 {deleteUser?.username} 吗？此操作不可恢复。</DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="outline" onClick={() => setDeleteUser(null)}>取消</Button>
          <Button variant="destructive" disabled={remove.isPending} onClick={() => remove.mutate()}>
            {remove.isPending && <Spinner />}
            确认删除
          </Button>
        </DialogFooter>
      </Dialog>
    </div>
  );
}
