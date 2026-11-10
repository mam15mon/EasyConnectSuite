#!/usr/bin/env bash
set -euo pipefail
: "${ECS_HOST_NET_NS:?}" "${ECS_HOST_MNT_NS:?}" "${ECS_TEST_BIN:?}"
if [ "${ECS_RESOLVED_INNER:-}" != 1 ]; then
  [ "$(readlink /proc/self/ns/net)" != "$ECS_HOST_NET_NS" ]
  [ "$(readlink /proc/self/ns/mnt)" != "$ECS_HOST_MNT_NS" ]
  mount --make-rprivate /
  mount -t tmpfs -o mode=755 tmpfs /run
  mkdir -p /run/systemd
  unset XDG_RUNTIME_DIR
  mount -t tmpfs -o mode=1777 tmpfs /tmp
  mount --bind /etc /etc
  mount -o remount,bind,ro /etc
  exec dbus-run-session -- env ECS_RESOLVED_INNER=1 bash "$0"
fi
export DBUS_SYSTEM_BUS_ADDRESS="$DBUS_SESSION_BUS_ADDRESS"
export ECS_REAL_RESOLVED_TEST=1
export SYSTEMD_LOG_LEVEL=debug
/usr/lib/systemd/systemd-resolved > /tmp/resolved-test.log 2>&1 &
resolved_pid=$!
cleanup() { kill "$resolved_pid" 2>/dev/null || true; wait "$resolved_pid" 2>/dev/null || true; }
trap cleanup EXIT
ready=0
for _ in $(seq 1 100); do
  if busctl --address="$DBUS_SESSION_BUS_ADDRESS" --no-legend list | grep -q '^org.freedesktop.resolve1[[:space:]]'; then ready=1; break; fi
  if ! kill -0 "$resolved_pid" 2>/dev/null; then break; fi
  sleep .1
done
if [ "$ready" -ne 1 ]; then busctl --address="$DBUS_SESSION_BUS_ADDRESS" --no-legend list >&2 || true; cat /tmp/resolved-test.log >&2; echo 'isolated resolved did not start' >&2; exit 1; fi
"$ECS_TEST_BIN" -test.run '^TestResolvedRealIsolatedLinkLifecycle$' -test.v
