#!/usr/bin/env bash
set -euo pipefail

work_dir=$(mktemp -d)
gateway_pid=
reject_pid=
tcp_pid=
client_pid=
cleanup() {
  if [ -n "$client_pid" ]; then kill "$client_pid" 2>/dev/null || true; wait "$client_pid" 2>/dev/null || true; fi
  if [ -n "$gateway_pid" ]; then kill "$gateway_pid" 2>/dev/null || true; wait "$gateway_pid" 2>/dev/null || true; fi
  if [ -n "$reject_pid" ]; then kill "$reject_pid" 2>/dev/null || true; wait "$reject_pid" 2>/dev/null || true; fi
  if [ -n "$tcp_pid" ]; then sudo pkill -P "$tcp_pid" 2>/dev/null || true; sudo kill "$tcp_pid" 2>/dev/null || true; wait "$tcp_pid" 2>/dev/null || true; fi
  rm -rf "$work_dir"
}
trap cleanup EXIT

sudo python3 build/android/tcp-gateway.py >"$work_dir/tcp-gateway.log" 2>&1 &
tcp_pid=$!
for attempt in $(seq 1 100); do
  if grep -q tcp_gateway_ready "$work_dir/tcp-gateway.log"; then break; fi
  sleep .1
done
if ! grep -q tcp_gateway_ready "$work_dir/tcp-gateway.log"; then
  cat "$work_dir/tcp-gateway.log"
  exit 1
fi

go build -o "$work_dir/gateway" build/android/success-gateway.go
"$work_dir/gateway" "$work_dir/close-receive" "$work_dir/expire-session" >"$work_dir/gateway.log" 2>&1 &
gateway_pid=$!
python3 - <<'PY'
import socket, time
for _ in range(100):
    try:
        with socket.create_connection(('127.0.0.1', 18445), timeout=.2):
            break
    except OSError:
        time.sleep(.1)
else:
    raise SystemExit('mock gateway did not start')
PY
printf '%s\n' 'headless-private-canary' | "$RUNNER_TEMP/vpn-headless-linux-amd64" -server 127.0.0.1 -port 18445 -username mock-user -socks-port 19080 -http-port 19081 -proxy-all >"$work_dir/client.log" 2>&1 &
client_pid=$!
if ! python3 - <<'PY'
import socket, time
for _ in range(300):
    try:
        with socket.create_connection(('127.0.0.1', 19080), timeout=.2) as sock:
            sock.sendall(b'\x05\x01\x00')
            if sock.recv(2) == b'\x05\x00':
                break
    except OSError:
        time.sleep(.1)
else:
    raise SystemExit('SOCKS proxy did not accept a handshake')
PY
then
  cat "$work_dir/client.log" "$work_dir/gateway.log"
  exit 1
fi
if ! python3 - <<'PY'
import socket, struct

def read_exact(sock, count):
    data = b''
    while len(data) < count:
        chunk = sock.recv(count - len(data))
        if not chunk:
            raise SystemExit('SOCKS connection closed early')
        data += chunk
    return data

with socket.create_connection(('127.0.0.1', 19080), timeout=10) as sock:
    sock.sendall(b'\x05\x01\x00')
    if read_exact(sock, 2) != b'\x05\x00':
        raise SystemExit('SOCKS negotiation failed')
    sock.sendall(b'\x05\x01\x00\x01' + socket.inet_aton('10.90.0.3') + struct.pack('!H', 12345))
    reply = read_exact(sock, 4)
    if reply[:2] != b'\x05\x00' or reply[3] != 1:
        raise SystemExit(f'SOCKS connect failed: {reply!r}')
    read_exact(sock, 6)
    payload = b'linux-headless-tcp-round-trip'
    sock.sendall(payload)
    if read_exact(sock, len(payload)) != payload:
        raise SystemExit('TCP echo payload mismatch')
with socket.create_connection(('127.0.0.1', 19081), timeout=10) as sock:
    sock.sendall(b'CONNECT 10.90.0.3:12345 HTTP/1.1\r\nHost: 10.90.0.3:12345\r\n\r\n')
    headers = b''
    while not headers.endswith(b'\r\n\r\n'):
        headers += read_exact(sock, 1)
        if len(headers) > 8192:
            raise SystemExit('HTTP CONNECT response headers too large')
    if not headers.startswith(b'HTTP/1.1 200 '):
        raise SystemExit(f'HTTP CONNECT failed: {headers!r}')
    payload = b'linux-headless-http-round-trip'
    sock.sendall(payload)
    if read_exact(sock, len(payload)) != payload:
        raise SystemExit('HTTP CONNECT echo payload mismatch')
print('SOCKS and HTTP CONNECT TCP payloads returned through the mock VPN gateway')
PY
then
  cat "$work_dir/client.log" "$work_dir/gateway.log" "$work_dir/tcp-gateway.log"
  exit 1
fi
if ! grep -q 'login=1' "$work_dir/gateway.log"; then
  echo 'mock gateway did not observe login' >&2
  exit 1
fi
kill -TERM "$client_pid"
wait "$client_pid"
client_pid=
if grep -q 'headless-private-canary' "$work_dir/client.log" "$work_dir/gateway.log"; then
  echo 'credential leaked to test logs' >&2
  exit 1
fi
python3 - <<'PY'
import socket
for port in (19080, 19081):
    with socket.socket() as sock:
        sock.settimeout(1)
        if sock.connect_ex(('127.0.0.1', port)) == 0:
            raise SystemExit(f'proxy port {port} remained open after exit')
print('Mock login, SOCKS/HTTP TCP round trips, signal cleanup and credential redaction passed')
PY

# Exercise the desktop application's profile and connection backend with the same mock gateway.
ECS_GUI_GATEWAY_TEST=1 go test -tags webkit2_41 . -run '^TestLinuxGUIBackendMockGateway$' -count=1 -timeout=90s -v >"$work_dir/gui-backend.log" 2>&1 || {
  cat "$work_dir/gui-backend.log" "$work_dir/gateway.log"
  exit 1
}
if grep -q 'gui-private-canary' "$work_dir/gui-backend.log" "$work_dir/gateway.log"; then
  echo 'GUI credential leaked to test logs' >&2
  exit 1
fi
cat "$work_dir/gui-backend.log"

# Verify an explicit rejection ends the real headless process without retries.
openssl req -x509 -newkey rsa:2048 -nodes -keyout "$work_dir/key.pem" \
  -out "$work_dir/cert.pem" -days 1 -subj '/CN=localhost' >/dev/null 2>&1
modulus=$(openssl rsa -in "$work_dir/key.pem" -noout -modulus | cut -d= -f2)
python3 build/android/reject-gateway.py "$work_dir/cert.pem" "$work_dir/key.pem" "$modulus" "$work_dir/rejections" >"$work_dir/reject.log" 2>&1 &
reject_pid=$!
python3 - <<'PY'
import socket, time
for _ in range(100):
    try:
        with socket.create_connection(('127.0.0.1', 18444), timeout=.2):
            break
    except OSError:
        time.sleep(.1)
else:
    raise SystemExit('reject gateway did not start')
PY
set +e
printf '%s\n' 'rejection-private-canary' | timeout 20s "$RUNNER_TEMP/vpn-headless-linux-amd64" \
  -server 127.0.0.1 -port 18444 -username mock-user -socks-port 19080 -http-port 19081 >"$work_dir/rejected-client.log" 2>&1
reject_status=$?
set -e
if [ "$reject_status" -eq 0 ] || [ "$reject_status" -eq 124 ] || [ "$reject_status" -eq 137 ]; then
  cat "$work_dir/rejected-client.log" "$work_dir/reject.log"
  echo "Authentication rejection returned unexpected status: $reject_status" >&2
  exit 1
fi
if [ ! -f "$work_dir/rejections" ] || [ "$(wc -l < "$work_dir/rejections")" -ne 1 ]; then
  cat "$work_dir/rejected-client.log" "$work_dir/reject.log"
  echo 'Expected exactly one rejected password attempt' >&2
  exit 1
fi
if grep -q 'rejection-private-canary' "$work_dir/rejected-client.log" "$work_dir/reject.log"; then
  echo 'Rejected credential leaked to test logs' >&2
  exit 1
fi
python3 - <<'PY'
import socket
for port in (19080, 19081):
    with socket.socket() as sock:
        sock.settimeout(1)
        if sock.connect_ex(('127.0.0.1', port)) == 0:
            raise SystemExit(f'proxy port {port} remained open after rejection')
print('Authentication rejection exited without retry, leaked credentials or open proxy ports')
PY

# A service manager may stop the process while its credential pipe is idle.
python3 - <<'PY'
import os, signal, subprocess
process = subprocess.Popen([os.path.join(os.environ['RUNNER_TEMP'], 'vpn-headless-linux-amd64'),
    '-server', '127.0.0.1', '-username', 'mock-user'], stdin=subprocess.PIPE,
    stdout=subprocess.PIPE, stderr=subprocess.PIPE)
try:
    import time
    time.sleep(.3)
    process.send_signal(signal.SIGTERM)
    try:
        process.wait(timeout=5)
    except subprocess.TimeoutExpired:
        raise SystemExit('headless process did not stop while waiting for credentials')
finally:
    if process.poll() is None:
        process.kill()
        process.wait()
    process.stdin.close()
    process.stdout.close()
    process.stderr.close()
print('SIGTERM stops a process waiting for credentials')
PY
