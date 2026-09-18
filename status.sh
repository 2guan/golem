#!/usr/bin/env bash
cd "$(dirname "$0")"

PID=$(pgrep -f "^./golem" || true)
if [ -n "$PID" ]; then
  echo "🟢 Golem 正在运行中，PID: $PID"
  echo "--- 最近 10 行日志 (golem.log) ---"
  tail -n 10 golem.log 2>/dev/null || echo "(暂无独立日志文件)"
else
  echo "🔴 Golem 未运行"
fi
