import { useState } from "react";
import { useQuery, keepPreviousData } from "@tanstack/react-query";
import { ChevronLeft, ChevronRight, Search } from "lucide-react";
import { logsApi, actionLabel, actionLabels, type LogFilter } from "@/api/logs";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select } from "@/components/ui/select";
import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Spinner } from "@/components/ui/spinner";

export function Logs() {
  const [filter, setFilter] = useState<LogFilter>({ page: 1, pageSize: 20 });
  const [actor, setActor] = useState("");
  const [action, setAction] = useState("");
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");

  const { data, isFetching } = useQuery({
    queryKey: ["logs", filter],
    queryFn: () => logsApi.list(filter),
    placeholderData: keepPreviousData,
  });

  const apply = () =>
    setFilter({ page: 1, pageSize: 20, actor: actor || undefined, action: action || undefined, from: from || undefined, to: to || undefined });

  const totalPages = data ? Math.max(1, Math.ceil(data.total / data.pageSize)) : 1;

  return (
    <div className="flex flex-col gap-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">操作日志</h1>
        <p className="mt-1 text-sm text-muted-foreground">所有用户的操作记录，全员可见</p>
      </div>

      <Card className="p-4">
        <div className="flex flex-wrap items-end gap-3">
          <div className="flex flex-col gap-1.5">
            <Label className="text-xs">操作人</Label>
            <Input className="h-9 w-40" placeholder="用户名" value={actor} onChange={(e) => setActor(e.target.value)} />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label className="text-xs">动作类型</Label>
            <Select className="h-9 w-44" value={action} onChange={(e) => setAction(e.target.value)}>
              <option value="">全部</option>
              {Object.keys(actionLabels).map((k) => (
                <option key={k} value={k}>
                  {actionLabels[k]}
                </option>
              ))}
            </Select>
          </div>
          <div className="flex flex-col gap-1.5">
            <Label className="text-xs">起始日期</Label>
            <Input type="date" className="h-9" value={from} onChange={(e) => setFrom(e.target.value)} />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label className="text-xs">结束日期</Label>
            <Input type="date" className="h-9" value={to} onChange={(e) => setTo(e.target.value)} />
          </div>
          <Button className="h-9" onClick={apply}>
            <Search className="h-4 w-4" />
            筛选
          </Button>
        </div>
      </Card>

      <Card>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>时间</TableHead>
              <TableHead>操作人</TableHead>
              <TableHead>IP</TableHead>
              <TableHead>动作</TableHead>
              <TableHead>详情</TableHead>
              <TableHead>结果</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {data && data.items.length > 0 ? (
              data.items.map((l) => (
                <TableRow key={l.id}>
                  <TableCell className="whitespace-nowrap text-sm text-muted-foreground">
                    {new Date(l.createdAt).toLocaleString("zh-CN")}
                  </TableCell>
                  <TableCell className="font-medium">{l.actorUsername || "-"}</TableCell>
                  <TableCell className="text-sm text-muted-foreground">{l.ip}</TableCell>
                  <TableCell>
                    <Badge variant="secondary">{actionLabel(l.action)}</Badge>
                  </TableCell>
                  <TableCell className="text-sm">{l.detail}</TableCell>
                  <TableCell>
                    {l.result === "ok" ? <Badge variant="success">成功</Badge> : <Badge variant="danger">失败</Badge>}
                  </TableCell>
                </TableRow>
              ))
            ) : (
              <TableRow>
                <TableCell colSpan={6} className="py-12 text-center text-sm text-muted-foreground">
                  {isFetching ? <Spinner className="mx-auto h-5 w-5 text-primary" /> : "暂无日志"}
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </Card>

      <div className="flex items-center justify-between text-sm text-muted-foreground">
        <span>共 {data?.total ?? 0} 条记录</span>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="icon"
            disabled={(filter.page ?? 1) <= 1}
            onClick={() => setFilter((f) => ({ ...f, page: (f.page ?? 1) - 1 }))}
          >
            <ChevronLeft className="h-4 w-4" />
          </Button>
          <span>
            第 {filter.page ?? 1} / {totalPages} 页
          </span>
          <Button
            variant="outline"
            size="icon"
            disabled={(filter.page ?? 1) >= totalPages}
            onClick={() => setFilter((f) => ({ ...f, page: (f.page ?? 1) + 1 }))}
          >
            <ChevronRight className="h-4 w-4" />
          </Button>
        </div>
      </div>
    </div>
  );
}
