#!/usr/bin/env bash
#
# 把面板原先的 Docker 命名卷数据一次性迁移到宿主磁盘（bind mount）目录。
#
# 背景：旧版 docker-compose.yml 用命名卷（paneldata / nginx-config / acme-webroot）
# 存放数据库、站点配置、证书等；新版改为直接 bind mount 到宿主磁盘的
# ${PANEL_DATA_ROOT:-./data} 目录。切换后新目录是空的，需要先把命名卷里的
# 数据搬过来，否则面板会当成全新安装。本脚本完成这次搬迁。
#
# 特性：
#   - 幂等：目标目录已有数据则跳过该项，绝不覆盖现有数据。
#   - 安全：源卷以只读挂载，整目录（含 panel.db / panel.db-wal / panel.db-shm
#     与 .jwt_secret 等隐藏文件）一并复制。
#   - 全新安装（没有旧命名卷）时自动跳过，不报错。
#
# 用法：
#   docker compose down                      # 先停服务，保证数据静止
#   ./scripts/migrate-volumes-to-disk.sh     # 迁移
#   docker compose up -d                     # 用新的磁盘目录启动
#
# 若同名后缀存在多个命名卷（曾在不同目录/项目名下跑过），用项目名消除歧义：
#   COMPOSE_PROJECT_NAME=<项目名> ./scripts/migrate-volumes-to-disk.sh
#
set -euo pipefail

# 切到仓库根目录（脚本在 scripts/ 下）
cd "$(dirname "$0")/.."

# 数据落盘根目录，需与 docker-compose.yml / .env 中的 PANEL_DATA_ROOT 一致。
# 优先级：环境变量 > .env 中的值 > 默认 ./data
DATA_ROOT="${PANEL_DATA_ROOT:-}"
if [ -z "$DATA_ROOT" ] && [ -f .env ]; then
  DATA_ROOT="$(grep -E '^[[:space:]]*PANEL_DATA_ROOT=' .env | tail -n1 | cut -d= -f2- | tr -d '"' | tr -d "'" || true)"
fi
DATA_ROOT="${DATA_ROOT:-./data}"

# 拒绝在服务运行时迁移（数据可能正在写入，复制会不一致）
if docker compose ps -q 2>/dev/null | grep -q .; then
  echo "错误：检测到容器仍在运行。请先执行 'docker compose down' 再迁移。" >&2
  exit 1
fi

echo "数据将迁移到：${DATA_ROOT%/}/{panel,nginx-config,acme-webroot}"
echo

migrated=0
# 旧命名卷后缀 -> 新目标子目录
for suffix in paneldata nginx-config acme-webroot; do
  case "$suffix" in
    paneldata)    sub="panel" ;;
    nginx-config) sub="nginx-config" ;;
    acme-webroot) sub="acme-webroot" ;;
  esac

  # 解析旧命名卷的真实名字（带项目名前缀，如 nginxpanel-lite_paneldata）
  vol=""
  if [ -n "${COMPOSE_PROJECT_NAME:-}" ] && \
     docker volume inspect "${COMPOSE_PROJECT_NAME}_${suffix}" >/dev/null 2>&1; then
    vol="${COMPOSE_PROJECT_NAME}_${suffix}"
  else
    matches="$(docker volume ls -q | grep -E "_${suffix}$" || true)"
    count="$(printf '%s' "$matches" | grep -c . || true)"
    if [ "$count" = "0" ]; then
      echo "跳过 ${suffix}：未找到旧命名卷（全新安装或已迁移）"
      continue
    elif [ "$count" != "1" ]; then
      echo "错误：发现多个匹配 _${suffix} 的卷，请用 COMPOSE_PROJECT_NAME=<项目名> 指定后重试：" >&2
      printf '  %s\n' $matches >&2
      exit 1
    fi
    vol="$matches"
  fi

  # 目标目录（取绝对路径供 docker -v 使用）
  dest="${DATA_ROOT%/}/${sub}"
  mkdir -p "$dest"
  destabs="$(cd "$dest" && pwd)"

  # 目标已有内容则跳过，避免覆盖
  if [ -n "$(ls -A "$destabs" 2>/dev/null || true)" ]; then
    echo "跳过 ${suffix}：目标 ${dest} 已有数据（不覆盖）"
    continue
  fi

  echo "迁移 ${vol}  ->  ${dest}"
  # 用临时容器把只读源卷整目录复制到目标（cp -a 保留权限/时间，/from/. 含隐藏文件）
  docker run --rm \
    -v "${vol}:/from:ro" \
    -v "${destabs}:/to" \
    alpine:3.20 sh -c 'cp -a /from/. /to/ 2>/dev/null || true'
  migrated=$((migrated + 1))
done

echo
if [ "$migrated" -gt 0 ]; then
  echo "迁移完成（${migrated} 项）。现在可以："
  echo "  docker compose up -d"
  echo
  echo "确认面板数据正常后，旧命名卷可手动删除回收空间："
  echo "  docker volume ls | grep -E '_(paneldata|nginx-config|acme-webroot)\$'"
  echo "  docker volume rm <卷名>..."
else
  echo "没有需要迁移的数据。可直接 docker compose up -d。"
fi
