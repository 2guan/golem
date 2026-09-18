#!/usr/bin/env bash
cd "$(dirname "$0")"

PID=$(pgrep -f "^./golem" || true)
if [ -z "$PID" ]; then
  echo "ℹ️ Golem 当前未在运行"
  exit 0
fi

echo "🛑 正在停止 Golem (PID: $PID)..."
kill "$PID" 2>/dev/null || true

for i in {1..5}; do
  if ! kill -0 "$PID" 2>/dev/null; then
    break
  fi
  sleep 0.5
done

if kill -0 "$PID" 2>/dev/null; then
  kill -9 "$PID" 2>/dev/null || true
fi

pkill -f "plugins/.*/golem_plugin_" 2>/dev/null || true

echo "✅ Golem 已停止"
