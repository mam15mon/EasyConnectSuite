#!/usr/bin/env bash
set -euo pipefail

work_dir=$(mktemp -d "${RUNNER_TEMP:?}/gui-interaction.XXXXXXXX")
app_pid=
cleanup() {
  if [ -n "$app_pid" ]; then kill "$app_pid" 2>/dev/null || true; wait "$app_pid" 2>/dev/null || true; fi
  rm -rf "$work_dir"
}
trap cleanup EXIT
export XDG_CONFIG_HOME="$work_dir/config"
export WEBKIT_DISABLE_COMPOSITING_MODE=1
build/bin/VPN >"$work_dir/app.log" 2>&1 &
app_pid=$!
window=
for attempt in $(seq 1 200); do
  window=$(xdotool search --onlyvisible --name '^VPN$' 2>/dev/null | head -n 1 || true)
  if [ -n "$window" ]; then break; fi
  if ! kill -0 "$app_pid" 2>/dev/null; then echo 'GUI exited before opening a window' >&2; exit 1; fi
  sleep .1
done
[ -n "$window" ] || { echo 'GUI window did not appear' >&2; exit 1; }
xdotool windowfocus --sync "$window"
sleep 1
# The new-profile button is in the fixed-width left panel at the top of the window.
xdotool mousemove --window "$window" 255 87 click 1
sleep .5
xdotool key ctrl+a
xdotool type --clearmodifiers 'ci-profile'
xdotool key Tab
xdotool type --clearmodifiers '127.0.0.1'
xdotool key Tab ctrl+a
xdotool type --clearmodifiers '18445'
xdotool key Tab
xdotool type --clearmodifiers 'mock-user'
xdotool key Tab
xdotool type --clearmodifiers 'synthetic-password'
xdotool key Tab
xdotool type --clearmodifiers '19180'
xdotool key Tab
xdotool type --clearmodifiers '19181'
# Move through the optional fields to the save button.
for _ in $(seq 1 6); do xdotool key Tab; done
xdotool key Return
config="$XDG_CONFIG_HOME/VPN/vpn_profiles.json"
saved=0
for attempt in $(seq 1 100); do
  if [ -f "$config" ] && python3 - "$config" <<'PY'
import json, sys
try:
    with open(sys.argv[1], encoding='utf-8') as stream:
        data = json.load(stream)
    expected = {'name': 'ci-profile', 'server': '127.0.0.1', 'port': 18445,
                'username': 'mock-user', 'password': 'synthetic-password',
                'socksPort': 19180, 'httpPort': 19181, 'protocol': 'easyconnect',
                'tunMode': False, 'disableProxy': False}
    ok = any(all(profile.get(key) == value for key, value in expected.items())
             for profile in data.get('profiles', []))
except (OSError, ValueError):
    ok = False
sys.exit(0 if ok else 1)
PY
  then
    echo 'GUI profile was saved through the window and Wails binding'
    saved=1
    break
  fi
  sleep .1
done
if [ "$saved" -ne 1 ]; then
  echo 'GUI profile was not saved with the expected values' >&2
  exit 1
fi
: "${ECS_MOCK_GATEWAY_LOG:?}"
# SaveProfile makes this profile active; allow the frontend to refresh before clicking Connect.
sleep 1
before=$(grep -c 'tcp_packet_forwarded' "$ECS_MOCK_GATEWAY_LOG" || true)
xdotool mousemove --window "$window" 1010 87 click 1
python3 - <<'PY'
import socket, struct, time

def read_exact(sock, count):
    data = b''
    while len(data) < count:
        part = sock.recv(count - len(data))
        if not part:
            raise RuntimeError('proxy closed early')
        data += part
    return data

for _ in range(300):
    try:
        sock = socket.create_connection(('127.0.0.1', 19180), timeout=.2)
        break
    except OSError:
        time.sleep(.1)
else:
    raise SystemExit('GUI connect did not start the SOCKS proxy')
with sock:
    sock.settimeout(15)
    sock.sendall(b'\x05\x01\x00')
    if read_exact(sock, 2) != b'\x05\x00':
        raise SystemExit('GUI SOCKS negotiation failed')
    sock.sendall(b'\x05\x01\x00\x01' + socket.inet_aton('10.90.0.3') + struct.pack('!H', 12345))
    reply = read_exact(sock, 4)
    if reply[:2] != b'\x05\x00' or reply[3] != 1:
        raise SystemExit('GUI SOCKS connect failed')
    read_exact(sock, 6)
    payload = b'window-gui-vpn-round-trip'
    sock.sendall(payload)
    if read_exact(sock, len(payload)) != payload:
        raise SystemExit('GUI VPN payload mismatch')
PY
after=$(grep -c 'tcp_packet_forwarded' "$ECS_MOCK_GATEWAY_LOG" || true)
if [ "$after" -le "$before" ]; then
  echo 'GUI traffic did not pass through the mock tunnel' >&2
  exit 1
fi
xdotool mousemove --window "$window" 1010 87 click 1
python3 - <<'PY'
import socket, time
for _ in range(100):
    with socket.socket() as sock:
        sock.settimeout(.2)
        if sock.connect_ex(('127.0.0.1', 19180)) != 0:
            break
    time.sleep(.1)
else:
    raise SystemExit('GUI disconnect did not close the SOCKS proxy')
PY
# Reconnect and close the actual window while the VPN is active.
sleep 1
xdotool mousemove --window "$window" 1010 87 click 1
python3 - <<'PY'
import socket, time
for _ in range(300):
    try:
        with socket.create_connection(('127.0.0.1', 19180), timeout=.2) as sock:
            sock.sendall(b'\x05\x01\x00')
            if sock.recv(2) == b'\x05\x00':
                break
    except OSError:
        time.sleep(.1)
else:
    raise SystemExit('GUI did not reconnect before window close')
PY
echo 'Sending window close to connected GUI'
xdotool windowclose "$window"
for attempt in $(seq 1 200); do
  if ! kill -0 "$app_pid" 2>/dev/null; then break; fi
  sleep .1
done
if kill -0 "$app_pid" 2>/dev/null; then
  echo 'GUI did not exit after closing its connected window' >&2
  exit 1
fi
exit_status=0
wait "$app_pid" || exit_status=$?
app_pid=
if [ "$exit_status" -ne 0 ]; then
  echo "GUI exited with status $exit_status after window close" >&2
  sed 's/synthetic-password/[redacted]/g' "$work_dir/app.log" >&2
  if [ -f "$XDG_CONFIG_HOME/VPN/logs/crash.log" ]; then
    sed 's/synthetic-password/[redacted]/g' "$XDG_CONFIG_HOME/VPN/logs/crash.log" >&2
  fi
  exit 1
fi
python3 - <<'PY'
import socket
for port in (19180, 19181):
    with socket.socket() as sock:
        sock.settimeout(.2)
        if sock.connect_ex(('127.0.0.1', port)) == 0:
            raise SystemExit(f'proxy port {port} remained open after GUI window close')
PY
if ! grep -rq 'operation=app_shutdown' "$XDG_CONFIG_HOME/VPN/logs"; then
  echo 'GUI shutdown operation was not recorded' >&2
  exit 1
fi
echo 'Connected GUI window closed, completed shutdown, and released proxy ports'
if grep -rq 'synthetic-password' "$XDG_CONFIG_HOME/VPN/logs" "$ECS_MOCK_GATEWAY_LOG"; then
  echo 'GUI credential leaked to test logs' >&2
  exit 1
fi
echo 'GUI connected, forwarded a TCP payload through the mock VPN, and disconnected'
