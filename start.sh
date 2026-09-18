#!/usr/bin/env bash
cd "$(dirname "$0")"

PID=$(pgrep -f "^./golem" || true)
if [ -n "$PID" ]; then
  echo "⚠️ Golem 已在运行中，PID: $PID"
  exit 0
fi

nohup ./golem >> golem.log 2>&1 &
NEW_PID=$!
sleep 1

if kill -0 "$NEW_PID" 2>/dev/null; then
  echo "✅ Golem 后台启动成功！PID: $NEW_PID"
  echo "📄 运行日志: $(pwd)/golem.log"
  echo "🔍 实时查看: tail -F golem.log"
else
  echo "❌ 启动失败，请查看日志:"
  tail -n 20 golem.log 2>/dev/null || true
fi
