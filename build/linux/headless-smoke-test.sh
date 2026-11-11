#!/usr/bin/env bash
set -euo pipefail

# This mode runs only inside fresh network and mount namespaces. The mock
# gateway has its own network namespace so a local route cannot bypass TUN.
if [ "${1:-}" = --tun-isolated ]; then
  : "${RUNNER_TEMP:?}" "${ECS_HOST_NET_NS:?}" "${ECS_HOST_MNT_NS:?}" "${ECS_MOCK_GATEWAY_BIN:?}"
  [ "$(readlink /proc/self/ns/net)" != "$ECS_HOST_NET_NS" ]
  [ "$(readlink /proc/self/ns/mnt)" != "$ECS_HOST_MNT_NS" ]
  mount --make-rprivate /
  mount -t tmpfs -o mode=755 tmpfs /run
  mount -t tmpfs -o mode=1777 tmpfs /tmp
  mount --bind /etc /etc
  mount -o remount,bind,ro /etc
  mkdir -p /run/dbus /run/systemd
  unset XDG_RUNTIME_DIR
  tun_work=$(mktemp -d "$RUNNER_TEMP/headless-tun.XXXXXX")
  client_pid= gateway_pid= tcp_pid= bus_pid= resolved_pid=
  cleanup_tun() {
    status=$?
    if [ "$status" -ne 0 ]; then
      for log in "$tun_work"/*.log; do [ ! -f "$log" ] || cat "$log" >&2; done
      ip address >&2
      ip route show table all >&2
    fi
    for pid in "$client_pid" "$gateway_pid" "$tcp_pid" "$resolved_pid" "$bus_pid"; do
      if [ -n "$pid" ]; then kill -KILL "$pid" 2>/dev/null || true; wait "$pid" 2>/dev/null || true; fi
    done
    ip netns del ecs-gateway 2>/dev/null || true
    rm -rf "$tun_work"
  }
  trap cleanup_tun EXIT
  ip link set lo up
  ip netns add ecs-gateway
  ip link add ecs-client type veth peer name ecs-uplink
  ip link set ecs-uplink netns ecs-gateway
  ip addr add 192.0.2.1/30 dev ecs-client
  ip link set ecs-client up
  ip netns exec ecs-gateway ip addr add 192.0.2.2/30 dev ecs-uplink
  ip netns exec ecs-gateway ip link set ecs-uplink up
  ip netns exec ecs-gateway ip link set lo up
  # Only the two documentation addresses are underlay-reachable. No default route.
  ip -o route show table all | sort >"$tun_work/routes-before"
  cat >"$tun_work/bus.conf" <<'BUS'
<!DOCTYPE busconfig PUBLIC "-//freedesktop//DTD D-Bus Bus Configuration 1.0//EN" "http://www.freedesktop.org/standards/dbus/1.0/busconfig.dtd">
<busconfig>
  <type>system</type>
  <listen>unix:path=/run/dbus/system_bus_socket</listen>
  <auth>EXTERNAL</auth>
  <policy context="default">
    <allow user="*"/><allow own="*"/><allow send_destination="*"/><allow receive_sender="*"/>
  </policy>
</busconfig>
BUS
  dbus-daemon --nofork --config-file="$tun_work/bus.conf" >"$tun_work/bus.log" 2>&1 &
  bus_pid=$!
  for _ in $(seq 1 100); do
    if [ -S /run/dbus/system_bus_socket ]; then break; fi
    kill -0 "$bus_pid"
    sleep .1
  done
  [ -S /run/dbus/system_bus_socket ]
  chmod 777 /run/dbus/system_bus_socket
  export DBUS_SYSTEM_BUS_ADDRESS=unix:path=/run/dbus/system_bus_socket
  export DBUS_SESSION_BUS_ADDRESS="$DBUS_SYSTEM_BUS_ADDRESS"
  /usr/lib/systemd/systemd-resolved >"$tun_work/resolved.log" 2>&1 &
  resolved_pid=$!
  for _ in $(seq 1 100); do
    if busctl --system --no-legend list | grep -q '^org.freedesktop.resolve1[[:space:]]'; then break; fi
    kill -0 "$resolved_pid"
    sleep .1
  done
  busctl --system --no-legend list | grep -q '^org.freedesktop.resolve1[[:space:]]'
  ip netns exec ecs-gateway python3 build/android/tcp-gateway.py >"$tun_work/tcp.log" 2>&1 &
  tcp_pid=$!
  for _ in $(seq 1 100); do
    if grep -q tcp_gateway_ready "$tun_work/tcp.log"; then break; fi
    kill -0 "$tcp_pid"
    sleep .1
  done
  grep -q tcp_gateway_ready "$tun_work/tcp.log"
  ip netns exec ecs-gateway env ECS_MOCK_TUN_RESOURCES=1 ECS_MOCK_LISTEN_ADDRESS=192.0.2.2:18445 \
    "$ECS_MOCK_GATEWAY_BIN" "$tun_work/close-receive" "$tun_work/expire-session" >"$tun_work/gateway.log" 2>&1 &
  gateway_pid=$!
  python3 - <<'PY'
import socket, time
for _ in range(100):
    try:
        with socket.create_connection(('192.0.2.2', 18445), timeout=.2):
            break
    except OSError:
        time.sleep(.1)
else:
    raise SystemExit('isolated mock gateway did not start')
PY
  for mode in proxy no-proxy crash; do
    proxy_args=()
    if [ "$mode" != proxy ]; then proxy_args=(-no-proxy); fi
    printf '%s\n' 'headless-tun-private-canary' | "$RUNNER_TEMP/vpn-headless-linux-amd64" \
      -server 192.0.2.2 -port 18445 -username mock-user -tun "${proxy_args[@]}" \
      -socks-port 19080 -http-port 19081 >"$tun_work/client-$mode.log" 2>&1 &
    client_pid=$!
    for _ in $(seq 1 300); do
      if grep -q '^TUN: on$' "$tun_work/client-$mode.log"; then break; fi
      kill -0 "$client_pid"
      sleep .1
    done
    grep -q '^TUN: on$' "$tun_work/client-$mode.log"
    iface=$(ip -j route get 10.90.0.3 | python3 -c 'import json,sys; print(json.load(sys.stdin)[0]["dev"])')
    [[ "$iface" == ecs* && "$iface" != ecs-client ]]
    ifindex=$(ip -j link show dev "$iface" | python3 -c 'import json,sys; print(json.load(sys.stdin)[0]["ifindex"])')
    resolvectl domain "$iface" | grep -q '~synthetic.internal'
    dns_ip=$(resolvectl dns "$iface" | awk '{print $NF}')
    [[ "$dns_ip" == 198.19.129.* ]]
    timeout 15s resolvectl query --type=A --cache=no synthetic.internal >"$tun_work/dns-$mode.log"
    grep -q '10.90.0.3' "$tun_work/dns-$mode.log"
    python3 - "$mode" "$dns_ip" <<'PY'
import socket, struct, sys
mode, dns_ip = sys.argv[1:]
def exact(sock, n):
    data = b''
    while len(data) < n:
        part = sock.recv(n - len(data))
        if not part:
            raise SystemExit('stream closed before complete payload')
        data += part
    return data

def echo(sock, payload):
    sock.sendall(payload)
    if exact(sock, len(payload)) != payload:
        raise SystemExit('TCP echo mismatch')

with socket.create_connection(('10.90.0.3', 12345), timeout=10) as sock:
    echo(sock, b'headless-system-tcp')
    sock.shutdown(socket.SHUT_WR)
    if sock.recv(1) != b'':
        raise SystemExit('TCP half-close did not propagate EOF')
with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as sock:
    sock.settimeout(10)
    sock.sendto(b'ecs-udp-test', ('10.90.0.3', 12345))
    data, source = sock.recvfrom(1024)
    if data != b'ecs-udp-test' or source != ('10.90.0.3', 12345):
        raise SystemExit('TUN UDP echo mismatch')
query = struct.pack('!6H', 0xECA1, 0x100, 1, 0, 0, 0) + b'\x09synthetic\x08internal\0' + struct.pack('!HH', 1, 1)
for protocol in ('udp', 'tcp'):
    with socket.socket(socket.AF_INET, socket.SOCK_DGRAM if protocol == 'udp' else socket.SOCK_STREAM) as sock:
        sock.settimeout(10)
        sock.connect((dns_ip, 53))
        if protocol == 'udp':
            sock.send(query)
            answer = sock.recv(4096)
        else:
            sock.sendall(struct.pack('!H', len(query)) + query)
            answer = exact(sock, struct.unpack('!H', exact(sock, 2))[0])
        ident, flags, _, count, _, _ = struct.unpack('!6H', answer[:12])
        if ident != 0xECA1 or flags & 15 or not count or socket.inet_aton('10.90.0.3') not in answer:
            raise SystemExit(f'TUN {protocol} DNS answer mismatch')
for port in (19080, 19081):
    if mode != 'proxy':
        with socket.socket() as sock:
            sock.settimeout(1)
            if sock.connect_ex(('127.0.0.1', port)) == 0:
                raise SystemExit('-no-proxy opened a listener')
        continue
    with socket.create_connection(('127.0.0.1', port), timeout=10) as sock:
        if port == 19080:
            sock.sendall(b'\x05\x01\x00')
            if exact(sock, 2) != b'\x05\x00':
                raise SystemExit('TUN coexistence SOCKS handshake failed')
            sock.sendall(b'\x05\x01\x00\x01' + socket.inet_aton('10.90.0.3') + struct.pack('!H', 12345))
            if exact(sock, 4) != b'\x05\x00\x00\x01':
                raise SystemExit('TUN coexistence SOCKS connect failed')
            exact(sock, 6)
        else:
            sock.sendall(b'CONNECT 10.90.0.3:12345 HTTP/1.1\r\nHost: 10.90.0.3:12345\r\n\r\n')
            headers = b''
            while not headers.endswith(b'\r\n\r\n'):
                headers += exact(sock, 1)
                if len(headers) > 8192:
                    raise SystemExit('HTTP response headers too large')
            if not headers.startswith(b'HTTP/1.1 200 '):
                raise SystemExit('TUN coexistence HTTP CONNECT failed')
        echo(sock, b'headless-proxy-coexistence')
print(f'{mode}: real TUN TCP/UDP, UDP/TCP DNS, resolved split DNS and proxy mode passed')
PY
    if [ "$mode" = proxy ]; then kill -TERM "$client_pid"; fi
    if [ "$mode" = no-proxy ]; then kill -INT "$client_pid"; fi
    if [ "$mode" = crash ]; then kill -KILL "$client_pid"; fi
    for _ in $(seq 1 150); do
      if ! kill -0 "$client_pid" 2>/dev/null; then break; fi
      sleep .1
    done
    if kill -0 "$client_pid" 2>/dev/null; then
      echo "headless $mode process did not finish signal cleanup within 15 seconds" >&2
      exit 1
    fi
    result=0
    wait "$client_pid" || result=$?
    client_pid=
    if [ "$mode" = crash ]; then [ "$result" -eq 137 ]; else [ "$result" -eq 0 ]; fi
    for _ in $(seq 1 100); do
      if ! ip link show dev "$iface" >/dev/null 2>&1 && ! resolvectl dns | grep -q "Link $ifindex "; then break; fi
      sleep .1
    done
    kill -0 "$resolved_pid"
    ! ip link show dev "$iface" >/dev/null 2>&1
    resolved_dns=$(resolvectl dns)
    ! grep -q "Link $ifindex " <<<"$resolved_dns"
    ip -o route show table all | sort >"$tun_work/routes-after"
    diff -u "$tun_work/routes-before" "$tun_work/routes-after"
    python3 - <<'PY'
import socket
for port in (19080, 19081):
    with socket.socket() as sock:
        sock.settimeout(1)
        if sock.connect_ex(('127.0.0.1', port)) == 0:
            raise SystemExit('proxy listener survived process exit')
PY
    echo "$mode: process exit removed its interface, routes, resolved link and listeners"
  done
  # The GUI backend test uses the same isolated real network and privileged Worker.
  ECS_LINUX_TUN_SYSTEM_TEST=1 ECS_GUI_TUN_GATEWAY_TEST=1 ECS_MOCK_GATEWAY_HOST=192.0.2.2 \
    ECS_MOCK_GATEWAY_LOG="$tun_work/gateway.log" "$RUNNER_TEMP/linux-worker-test" \
    -test.run '^TestLinuxGUIBackendTUNMockGateway$' -test.v -test.timeout=90s >"$tun_work/gui-tun.log" 2>&1
  cat "$tun_work/gui-tun.log"
  grep -q 'tcp_packet_forwarded' "$tun_work/gateway.log"
  grep -q 'udp_echo' "$tun_work/gateway.log"
  if grep -q 'private-canary' "$tun_work"/*.log; then
    echo 'credential leaked into isolated TUN logs' >&2
    exit 1
  fi
  echo 'Isolated headless and GUI TUN mock-gateway smoke tests passed'
  exit 0
fi

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
ECS_MOCK_GUI_RESOURCES="$work_dir/gui-resources" "$work_dir/gateway" "$work_dir/close-receive" "$work_dir/expire-session" >"$work_dir/gateway.log" 2>&1 &
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
touch "$work_dir/gui-resources"
tcp_before=$(grep -c 'tcp_packet_forwarded' "$work_dir/gateway.log" || true)
ECS_GUI_GATEWAY_TEST=1 ECS_MOCK_GATEWAY_LOG="$work_dir/gateway.log" go test -tags webkit2_41 . -run '^TestLinuxGUIBackendMockGateway$' -count=1 -timeout=90s -v >"$work_dir/gui-backend.log" 2>&1 || {
  cat "$work_dir/gui-backend.log" "$work_dir/gateway.log"
  exit 1
}
if grep -q 'gui-private-canary' "$work_dir/gui-backend.log" "$work_dir/gateway.log"; then
  echo 'GUI credential leaked to test logs' >&2
  exit 1
fi
cat "$work_dir/gui-backend.log"
tcp_after=$(grep -c 'tcp_packet_forwarded' "$work_dir/gateway.log" || true)
if [ "$tcp_after" -le "$tcp_before" ]; then
  echo 'GUI proxy traffic did not pass through the VPN tunnel' >&2
  exit 1
fi

# Exercise the actual window with the same isolated mock gateway.
ECS_MOCK_GATEWAY_LOG="$work_dir/gateway.log" timeout -k 5s 90s dbus-run-session -- xvfb-run -a bash build/linux/gui-interaction-test.sh

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

# Run system TUN checks with independent client/gateway networks and private DNS.
cp "$work_dir/gateway" "$RUNNER_TEMP/tun-smoke-gateway"
sudo env RUNNER_TEMP="$RUNNER_TEMP" ECS_HOST_NET_NS="$(readlink /proc/self/ns/net)" \
  ECS_HOST_MNT_NS="$(readlink /proc/self/ns/mnt)" ECS_MOCK_GATEWAY_BIN="$RUNNER_TEMP/tun-smoke-gateway" \
  timeout -k 5s 240s unshare --mount --net bash build/linux/headless-smoke-test.sh --tun-isolated
