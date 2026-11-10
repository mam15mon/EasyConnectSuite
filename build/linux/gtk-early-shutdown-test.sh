#!/usr/bin/env bash
set -eu
work="$(mktemp -d "$RUNNER_TEMP/gtk-gate.XXXXXX")"
app_pid=
cleanup() {
  if [ -n "$app_pid" ]; then kill -KILL "$app_pid" 2>/dev/null || true; wait "$app_pid" 2>/dev/null || true; fi
  rm -rf "$work"
}
trap cleanup EXIT
cc -shared -fPIC -o "$work/gate.so" build/linux/gtk-main-gate.c -ldl
mkfifo "$work/gate"
export XDG_CONFIG_HOME="$work/config" WEBKIT_DISABLE_COMPOSITING_MODE=1 ECS_GTK_TEST_GATE="$work/gate"
LD_PRELOAD="$work/gate.so" build/bin/VPN >"$work/output.log" 2>&1 &
app_pid=$!
for i in $(seq 1 200); do
  if grep -q GTK_TEST_MAIN_PAUSED "$work/output.log"; then break; fi
  if ! kill -0 "$app_pid" 2>/dev/null; then cat "$work/output.log"; exit 1; fi
  sleep .1
done
if ! grep -q GTK_TEST_MAIN_PAUSED "$work/output.log"; then cat "$work/output.log"; echo "GTK main loop was not reached" >&2; exit 1; fi
kill -TERM "$app_pid"
log="$XDG_CONFIG_HOME/VPN/logs/crash.log"
for i in $(seq 1 100); do
  if [ -f "$log" ] && grep -q 'stage=wait_gtk_main_loop' "$log"; then break; fi
  if ! kill -0 "$app_pid" 2>/dev/null; then cat "$work/output.log"; exit 1; fi
  sleep .1
done
if [ ! -f "$log" ] || ! grep -q 'stage=wait_gtk_main_loop' "$log"; then cat "$work/output.log"; echo "shutdown did not wait for GTK" >&2; exit 1; fi
printf x >"$work/gate"
wait "$app_pid"
app_pid=
if grep -q 'gtk_main_quit: assertion' "$work/output.log"; then cat "$work/output.log"; exit 1; fi
if ! grep -q 'name=linux_close.*phase=end outcome=ready' "$log"; then cat "$log"; exit 1; fi
echo "Early shutdown waited for GTK main loop and completed"
