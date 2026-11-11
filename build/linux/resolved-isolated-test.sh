#!/usr/bin/env bash
set -euo pipefail
: "${ECS_HOST_NET_NS:?}" "${ECS_HOST_MNT_NS:?}" "${ECS_TEST_BIN:?}"
[ "$(readlink /proc/self/ns/net)" != "$ECS_HOST_NET_NS" ]
[ "$(readlink /proc/self/ns/mnt)" != "$ECS_HOST_MNT_NS" ]
mount --make-rprivate /
mount -t tmpfs -o mode=755 tmpfs /run
mkdir -p /run/systemd /run/dbus
unset XDG_RUNTIME_DIR
mount -t tmpfs -o mode=1777 tmpfs /tmp
mount --bind /etc /etc
mount -o remount,bind,ro /etc
cat > /tmp/private-bus.conf <<'EOF'
<!DOCTYPE busconfig PUBLIC "-//freedesktop//DTD D-BUS Bus Configuration 1.0//EN" "http://www.freedesktop.org/standards/dbus/1.0/busconfig.dtd">
<busconfig>
  <type>system</type>
  <listen>unix:path=/run/dbus/system_bus_socket</listen>
  <auth>EXTERNAL</auth>
  <policy context="default">
    <allow user="*"/>
    <allow own="*"/>
    <allow send_destination="*"/>
    <allow receive_sender="*"/>
  </policy>
</busconfig>
EOF
dbus-daemon --nofork --config-file=/tmp/private-bus.conf > /tmp/dbus-test.log 2>&1 &
bus_pid=$!
resolved_pid=
cleanup() {
  status=$?
  if [ "$status" -ne 0 ]; then cat /tmp/resolved-test.log /tmp/dbus-test.log >&2; fi
  if [ -n "$resolved_pid" ]; then kill "$resolved_pid" 2>/dev/null || true; wait "$resolved_pid" 2>/dev/null || true; fi
  kill "$bus_pid" 2>/dev/null || true
  wait "$bus_pid" 2>/dev/null || true
}
trap cleanup EXIT
for _ in $(seq 1 100); do
  if [ -S /run/dbus/system_bus_socket ]; then break; fi
  if ! kill -0 "$bus_pid" 2>/dev/null; then cat /tmp/dbus-test.log >&2; exit 1; fi
  sleep .1
done
[ -S /run/dbus/system_bus_socket ] || { echo 'private bus did not start' >&2; exit 1; }
chmod 777 /run/dbus/system_bus_socket
export DBUS_SYSTEM_BUS_ADDRESS=unix:path=/run/dbus/system_bus_socket
export DBUS_SESSION_BUS_ADDRESS="$DBUS_SYSTEM_BUS_ADDRESS"
export ECS_REAL_RESOLVED_TEST=1
/usr/lib/systemd/systemd-resolved > /tmp/resolved-test.log 2>&1 &
resolved_pid=$!
export ECS_TEST_RESOLVED_PID="$resolved_pid"
ready=0
for _ in $(seq 1 100); do
  if busctl --address="$DBUS_SESSION_BUS_ADDRESS" --no-legend list | grep -q '^org.freedesktop.resolve1[[:space:]]'; then ready=1; break; fi
  if ! kill -0 "$resolved_pid" 2>/dev/null; then break; fi
  sleep .1
done
if [ "$ready" -ne 1 ]; then busctl --address="$DBUS_SESSION_BUS_ADDRESS" --no-legend list >&2 || true; cat /tmp/resolved-test.log >&2; echo 'isolated resolved did not start' >&2; exit 1; fi
"$ECS_TEST_BIN" -test.run '^TestResolvedRealIsolated' -test.timeout=90s -test.v
