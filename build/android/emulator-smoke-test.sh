#!/usr/bin/env bash
set -euo pipefail

adb push "$RUNNER_TEMP/android-tun-test" /data/local/tmp/android-tun-test
adb shell chmod 755 /data/local/tmp/android-tun-test
adb shell /data/local/tmp/android-tun-test -test.v -test.timeout=30s
adb install -r android-app/app/build/outputs/apk/debug/app-debug.apk
adb shell am start -W -n com.zju.easyconnect.suite/.MainActivity
adb shell uiautomator dump /sdcard/window.xml >/dev/null
adb exec-out cat /sdcard/window.xml > "$RUNNER_TEMP/android-window.xml"
python3 - "$RUNNER_TEMP/android-window.xml" <<'PY'
import sys
import xml.etree.ElementTree as ET

root = ET.parse(sys.argv[1]).getroot()
ids = {node.get("resource-id") for node in root.iter("node")}
required = {"etServer", "etUsername", "etPassword", "btnToggle", "tvStatus"}
missing = {item for item in required if f"com.zju.easyconnect.suite:id/{item}" not in ids}
if missing:
    raise SystemExit(f"Missing UI elements: {sorted(missing)}")
status = next(node for node in root.iter("node") if node.get("resource-id") == "com.zju.easyconnect.suite:id/tvStatus")
if status.get("text") != "未连接":
    raise SystemExit(f"Unexpected initial status: {status.get('text')!r}")
print("App launched with the expected fields and disconnected status")
PY

# Recreate the activity through rotation and verify its state is restored.
adb shell settings put system accelerometer_rotation 0
adb shell settings put system user_rotation 1
adb shell settings put system user_rotation 0
adb shell input swipe 500 1500 500 400 400
adb shell uiautomator dump /sdcard/window.xml >/dev/null
adb exec-out cat /sdcard/window.xml > "$RUNNER_TEMP/android-window.xml"
python3 - "$RUNNER_TEMP/android-window.xml" <<'PY'
import sys
import xml.etree.ElementTree as ET
root = ET.parse(sys.argv[1]).getroot()
status = next(node for node in root.iter("node") if node.get("resource-id") == "com.zju.easyconnect.suite:id/tvStatus")
if status.get("text") != "未连接":
    raise SystemExit(f"Unexpected status after activity recreation: {status.get('text')!r}")
print("Disconnected state is restored after rotation")
PY

# Scroll to the connection button, then verify empty input is rejected.
adb shell input swipe 500 1500 500 400 400
adb shell uiautomator dump /sdcard/window.xml >/dev/null
adb exec-out cat /sdcard/window.xml > "$RUNNER_TEMP/android-window.xml"
coordinates=$(python3 - "$RUNNER_TEMP/android-window.xml" <<'PY'
import re
import sys
import xml.etree.ElementTree as ET

root = ET.parse(sys.argv[1]).getroot()
button = next(node for node in root.iter("node") if node.get("resource-id") == "com.zju.easyconnect.suite:id/btnToggle")
left, top, right, bottom = map(int, re.findall(r"\d+", button.get("bounds")))
print((left + right) // 2, (top + bottom) // 2)
PY
)
adb shell input tap $coordinates
adb shell uiautomator dump /sdcard/window.xml >/dev/null
adb exec-out cat /sdcard/window.xml > "$RUNNER_TEMP/android-window.xml"
python3 - "$RUNNER_TEMP/android-window.xml" <<'PY'
import sys
import xml.etree.ElementTree as ET

root = ET.parse(sys.argv[1]).getroot()
status = next(node for node in root.iter("node") if node.get("resource-id") == "com.zju.easyconnect.suite:id/tvStatus")
if status.get("text") != "服务器地址和账号不能为空":
    raise SystemExit(f"Unexpected validation status: {status.get('text')!r}")
print("Empty server and account are rejected")
PY

# With valid-looking fields, Android must request VPN consent before starting the service.
tap_field() {
  adb shell uiautomator dump /sdcard/window.xml >/dev/null
  adb exec-out cat /sdcard/window.xml > "$RUNNER_TEMP/android-window.xml"
  local coords
  coords=$(python3 - "$RUNNER_TEMP/android-window.xml" "$1" <<'PY'
import re
import sys
import xml.etree.ElementTree as ET
root = ET.parse(sys.argv[1]).getroot()
node = next(node for node in root.iter("node") if node.get("resource-id") == "com.zju.easyconnect.suite:id/" + sys.argv[2])
left, top, right, bottom = map(int, re.findall(r"\d+", node.get("bounds")))
print((left + right) // 2, (top + bottom) // 2)
PY
  )
  adb shell input tap $coords
}
adb shell input swipe 500 400 500 1500 400
tap_field etServer
adb shell input text 127.0.0.1
adb shell input keyevent 4
tap_field etUsername
adb shell input text testuser
adb shell input keyevent 4
adb shell input swipe 500 1500 500 400 400
tap_field btnToggle
adb shell uiautomator dump /sdcard/window.xml >/dev/null
adb exec-out cat /sdcard/window.xml > "$RUNNER_TEMP/android-window.xml"
python3 - "$RUNNER_TEMP/android-window.xml" <<'PY'
import sys
import xml.etree.ElementTree as ET
root = ET.parse(sys.argv[1]).getroot()
if not any(node.get("package") == "com.android.vpndialogs" for node in root.iter("node")):
    raise SystemExit("Android VPN authorization dialog was not displayed")
print("Android VPN authorization dialog appeared")
PY
coordinates=$(python3 - "$RUNNER_TEMP/android-window.xml" <<'PY'
import re
import sys
import xml.etree.ElementTree as ET
root = ET.parse(sys.argv[1]).getroot()
button = next(node for node in root.iter("node") if node.get("resource-id") == "android:id/button2")
left, top, right, bottom = map(int, re.findall(r"\d+", button.get("bounds")))
print((left + right) // 2, (top + bottom) // 2)
PY
)
adb shell input tap $coordinates
adb shell input swipe 500 1500 500 400 400
adb shell uiautomator dump /sdcard/window.xml >/dev/null
adb exec-out cat /sdcard/window.xml > "$RUNNER_TEMP/android-window.xml"
python3 - "$RUNNER_TEMP/android-window.xml" <<'PY'
import sys
import xml.etree.ElementTree as ET
root = ET.parse(sys.argv[1]).getroot()
status = next(node for node in root.iter("node") if node.get("resource-id") == "com.zju.easyconnect.suite:id/tvStatus")
if status.get("text") != "VPN 授权被取消":
    raise SystemExit(f"Unexpected status after denying authorization: {status.get('text')!r}")
print("VPN authorization denial returned to the app")
PY

# A local stalled TLS peer keeps authentication in progress until it is cancelled.
python3 -u - "$RUNNER_TEMP/mock-accepted" "$RUNNER_TEMP/mock-closed" <<'PY' &
import socket
import sys

with socket.socket() as server:
    server.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    server.bind(("127.0.0.1", 18443))
    server.listen()
    for index in range(2):
        connection, _ = server.accept()
        with connection:
            connection.settimeout(30)
            open(sys.argv[1] + str(index), "w").close()
            while connection.recv(4096):
                pass
            open(sys.argv[2] + str(index), "w").close()
PY
mock_pid=$!
trap 'kill "$mock_pid" 2>/dev/null || true' EXIT
adb reverse tcp:18443 tcp:18443
adb shell input swipe 500 400 500 1500 400
tap_field etPort
adb shell input keyevent 123
adb shell input keyevent 67 67 67
adb shell input text 18443
adb shell input keyevent 4
adb shell input swipe 500 1500 500 400 400
tap_field btnToggle
adb shell uiautomator dump /sdcard/window.xml >/dev/null
adb exec-out cat /sdcard/window.xml > "$RUNNER_TEMP/android-window.xml"
coordinates=$(python3 - "$RUNNER_TEMP/android-window.xml" <<'PY'
import re
import sys
import xml.etree.ElementTree as ET
root = ET.parse(sys.argv[1]).getroot()
button = next(node for node in root.iter("node") if node.get("resource-id") == "android:id/button1")
left, top, right, bottom = map(int, re.findall(r"\d+", button.get("bounds")))
print((left + right) // 2, (top + bottom) // 2)
PY
)
adb shell input tap $coordinates
for attempt in $(seq 1 30); do
  if [ -f "$RUNNER_TEMP/mock-accepted0" ]; then break; fi
  sleep 1
done
if [ ! -f "$RUNNER_TEMP/mock-accepted0" ]; then
  echo "Mock gateway did not receive the connection" >&2
  exit 1
fi
adb shell input keyevent KEYCODE_HOME
adb shell dumpsys activity services com.zju.easyconnect.suite/.TunnelService > "$RUNNER_TEMP/android-service.txt"
if ! grep -q 'isForeground=true' "$RUNNER_TEMP/android-service.txt"; then
  cat "$RUNNER_TEMP/android-service.txt"
  echo 'VPN service is not foreground while authentication is pending in the background' >&2
  exit 1
fi
adb shell am start -n com.zju.easyconnect.suite/.MainActivity >/dev/null
adb shell input keyevent KEYCODE_BACK
adb shell input swipe 160 550 160 100 400
adb shell input swipe 500 1500 500 400 400
tap_field btnToggle
# Start a second session immediately after cancellation.
tap_field btnToggle
for attempt in $(seq 1 30); do
  if [ -f "$RUNNER_TEMP/mock-accepted1" ]; then break; fi
  sleep 1
done
if [ ! -f "$RUNNER_TEMP/mock-accepted1" ]; then
  echo 'Second session did not reach the mock gateway' >&2
  exit 1
fi
sleep 2
if [ -f "$RUNNER_TEMP/mock-closed1" ]; then
  echo 'Second session closed before cancellation' >&2
  exit 1
fi
tap_field btnToggle
adb shell uiautomator dump /sdcard/window.xml >/dev/null
adb exec-out cat /sdcard/window.xml > "$RUNNER_TEMP/android-window.xml"
python3 - "$RUNNER_TEMP/android-window.xml" <<'PY'
import sys
import xml.etree.ElementTree as ET
root = ET.parse(sys.argv[1]).getroot()
status = next(node for node in root.iter("node") if node.get("resource-id") == "com.zju.easyconnect.suite:id/tvStatus")
if status.get("text") != "未连接":
    raise SystemExit(f"Unexpected status after cancellation: {status.get('text')!r}")
print("Connection cancellation returned to disconnected state")
PY

for attempt in $(seq 1 15); do
  if [ -f "$RUNNER_TEMP/mock-closed0" ]; then break; fi
  sleep 1
done
if [ ! -f "$RUNNER_TEMP/mock-closed0" ]; then
  echo "Connection remained open after cancellation" >&2
  exit 1
fi
for attempt in $(seq 1 15); do
  if [ -f "$RUNNER_TEMP/mock-closed1" ]; then break; fi
  sleep 1
done
if [ ! -f "$RUNNER_TEMP/mock-closed1" ]; then
  echo 'Second session remained open after cancellation' >&2
  exit 1
fi
printf '%s\n' 'Both authentication sessions closed after their respective cancellations'

# Confirm an explicit authentication rejection stops retries.
openssl req -x509 -newkey rsa:2048 -nodes -keyout "$RUNNER_TEMP/mock-key.pem" \
  -out "$RUNNER_TEMP/mock-cert.pem" -days 1 -subj '/CN=localhost' >/dev/null 2>&1
modulus=$(openssl rsa -in "$RUNNER_TEMP/mock-key.pem" -noout -modulus | cut -d= -f2)
python3 build/android/reject-gateway.py "$RUNNER_TEMP/mock-cert.pem" "$RUNNER_TEMP/mock-key.pem" "$modulus" "$RUNNER_TEMP/mock-rejections" &
reject_pid=$!
trap 'kill "$mock_pid" "$reject_pid" 2>/dev/null || true' EXIT
adb reverse tcp:18444 tcp:18444
adb shell input swipe 500 400 500 1500 400
tap_field etPort
adb shell input keyevent 123
adb shell input keyevent 67 67 67 67 67
adb shell input text 18444
adb shell input keyevent 4
adb shell input swipe 500 1500 500 400 400
tap_field btnToggle
for attempt in $(seq 1 30); do
  if [ -s "$RUNNER_TEMP/mock-rejections" ]; then break; fi
  sleep 1
done
if [ ! -s "$RUNNER_TEMP/mock-rejections" ]; then
  echo 'Mock gateway did not receive password authentication' >&2
  exit 1
fi
sleep 8
if [ "$(wc -l < "$RUNNER_TEMP/mock-rejections")" -ne 1 ]; then
  echo 'Authentication rejection was retried' >&2
  exit 1
fi
adb shell uiautomator dump /sdcard/window.xml >/dev/null
adb exec-out cat /sdcard/window.xml > "$RUNNER_TEMP/android-window.xml"
python3 - "$RUNNER_TEMP/android-window.xml" <<'PY'
import sys
import xml.etree.ElementTree as ET
root = ET.parse(sys.argv[1]).getroot()
status = next(node for node in root.iter("node") if node.get("resource-id") == "com.zju.easyconnect.suite:id/tvStatus")
if status.get("text") != "出错:认证被服务器拒绝":
    raise SystemExit(f"Unexpected rejection status: {status.get('text')!r}")
print("Explicit rejection stopped retries and is shown to the user")
PY

# Complete authentication and establish a real emulator VPN interface.
sudo python3 build/android/tcp-gateway.py > "$RUNNER_TEMP/tcp-gateway.log" 2>&1 &
tcp_pid=$!
trap 'kill "$mock_pid" "$reject_pid" "${success_pid:-}" 2>/dev/null || true; sudo kill "$tcp_pid" 2>/dev/null || true' EXIT
for attempt in $(seq 1 15); do
  if grep -q tcp_gateway_ready "$RUNNER_TEMP/tcp-gateway.log"; then break; fi
  sleep 1
done
if ! grep -q tcp_gateway_ready "$RUNNER_TEMP/tcp-gateway.log"; then
  cat "$RUNNER_TEMP/tcp-gateway.log"
  exit 1
fi
go run build/android/success-gateway.go "$RUNNER_TEMP/close-receive" "$RUNNER_TEMP/expire-session" > "$RUNNER_TEMP/success-gateway.log" 2>&1 &
success_pid=$!
adb reverse tcp:18445 tcp:18445
adb shell input swipe 500 400 500 1500 400
tap_field etPort
adb shell input keyevent 123
adb shell input keyevent 67 67 67 67 67
adb shell input text 18445
adb shell input keyevent 4
adb shell input swipe 500 1500 500 400 400
tap_field btnToggle
for attempt in $(seq 1 45); do
  if grep -q 'handshake=6' "$RUNNER_TEMP/success-gateway.log"; then break; fi
  sleep 1
done
if ! grep -q 'handshake=6' "$RUNNER_TEMP/success-gateway.log"; then
  cat "$RUNNER_TEMP/success-gateway.log"
  echo 'VPN receive channel was not established' >&2
  exit 1
fi
adb shell uiautomator dump /sdcard/window.xml >/dev/null
adb exec-out cat /sdcard/window.xml > "$RUNNER_TEMP/android-window.xml"
python3 - "$RUNNER_TEMP/android-window.xml" <<'PY'
import sys
import xml.etree.ElementTree as ET
root = ET.parse(sys.argv[1]).getroot()
status = next(node for node in root.iter("node") if node.get("resource-id") == "com.zju.easyconnect.suite:id/tvStatus")
if status.get("text") != "已连接 · 全部流量经 VPN":
    raise SystemExit(f"Unexpected connected status: {status.get('text')!r}")
print("Mock gateway authenticated and established Android VPN")
PY
adb push "$RUNNER_TEMP/android-udp-probe" /data/local/tmp/android-udp-probe
adb shell chmod 755 /data/local/tmp/android-udp-probe
if ! adb shell /data/local/tmp/android-udp-probe; then
  cat "$RUNNER_TEMP/success-gateway.log"
  echo 'UDP payload did not return through VPN' >&2
  exit 1
fi
touch "$RUNNER_TEMP/close-receive"
# The first receive stream closes after the first echo. Wait for a new receive
# stream and confirm another datagram traverses the recovered channel.
for attempt in $(seq 1 45); do
  if [ "$(grep -c 'handshake=6' "$RUNNER_TEMP/success-gateway.log" || true)" -ge 2 ]; then break; fi
  sleep 1
done
if [ "$(grep -c 'handshake=6' "$RUNNER_TEMP/success-gateway.log" || true)" -lt 2 ]; then
  cat "$RUNNER_TEMP/success-gateway.log"
  echo 'Receive channel did not recover after server close' >&2
  exit 1
fi
if ! adb shell /data/local/tmp/android-udp-probe; then
  cat "$RUNNER_TEMP/success-gateway.log"
  echo 'UDP failed after receive channel recovery' >&2
  exit 1
fi
echo 'UDP recovered after server receive stream closed'
adb push "$RUNNER_TEMP/android-tcp-probe" /data/local/tmp/android-tcp-probe
adb shell chmod 755 /data/local/tmp/android-tcp-probe
if ! adb shell /data/local/tmp/android-tcp-probe; then
  cat "$RUNNER_TEMP/tcp-gateway.log" "$RUNNER_TEMP/success-gateway.log"
  echo 'TCP payload did not return through VPN' >&2
  exit 1
fi
touch "$RUNNER_TEMP/expire-session"
for attempt in $(seq 1 60); do
  if grep -q 'login=2' "$RUNNER_TEMP/success-gateway.log" && [ "$(grep -c 'handshake=6' "$RUNNER_TEMP/success-gateway.log" || true)" -ge 3 ]; then break; fi
  sleep 1
done
if ! grep -q 'login=2' "$RUNNER_TEMP/success-gateway.log" || [ "$(grep -c 'handshake=6' "$RUNNER_TEMP/success-gateway.log" || true)" -lt 3 ]; then
  cat "$RUNNER_TEMP/success-gateway.log"
  adb logcat -d -s VPN:I GoLog:I AndroidRuntime:E
  echo 'VPN did not reauthenticate after session expiration' >&2
  exit 1
fi
if ! adb shell /data/local/tmp/android-udp-probe; then
  cat "$RUNNER_TEMP/success-gateway.log"
  echo 'UDP failed after reauthentication' >&2
  exit 1
fi
echo 'UDP recovered after session reauthentication'
if ! adb shell /data/local/tmp/android-tcp-probe; then
  echo 'TCP failed after reauthentication' >&2
  exit 1
fi
# Leave the app in the background while the VPN service continues forwarding.
adb shell input keyevent KEYCODE_HOME
sleep 20
if ! adb shell /data/local/tmp/android-udp-probe || ! adb shell /data/local/tmp/android-tcp-probe; then
  adb logcat -d -s VPN:I GoLog:I AndroidRuntime:E
  echo 'VPN traffic failed while the app was in the background' >&2
  exit 1
fi
echo 'UDP and TCP remained available while app was in background'
adb shell am start -n com.zju.easyconnect.suite/.MainActivity >/dev/null
adb shell input keyevent KEYCODE_BACK
adb shell input swipe 160 550 160 100 400
for attempt in $(seq 1 15); do
  adb shell uiautomator dump /sdcard/window.xml >/dev/null
  if adb exec-out cat /sdcard/window.xml | grep -q 'com.zju.easyconnect.suite:id/btnToggle'; then break; fi
  if [ "$attempt" -eq 15 ]; then
    adb exec-out cat /sdcard/window.xml
    adb shell dumpsys activity activities | grep -E 'mResumedActivity|topResumedActivity' || true
    echo 'App did not return to the foreground' >&2
    exit 1
  fi
  adb shell input swipe 160 550 160 100 400
  sleep 1
done
tap_field btnToggle
for attempt in $(seq 1 15); do
  adb shell uiautomator dump /sdcard/window.xml >/dev/null
  adb exec-out cat /sdcard/window.xml > "$RUNNER_TEMP/android-window.xml"
  if python3 - "$RUNNER_TEMP/android-window.xml" <<'PY'
import sys
import xml.etree.ElementTree as ET
root = ET.parse(sys.argv[1]).getroot()
status = next((node for node in root.iter("node") if node.get("resource-id") == "com.zju.easyconnect.suite:id/tvStatus"), None)
sys.exit(0 if status is not None and status.get("text") == "未连接" else 1)
PY
  then
    echo 'Android VPN disconnected'
    break
  fi
  if [ "$attempt" -eq 15 ]; then
    adb logcat -d -s VPN:I GoLog:I AndroidRuntime:E
    echo 'Android VPN did not disconnect within the test deadline' >&2
    exit 1
  fi
done
tap_field btnToggle
for attempt in $(seq 1 45); do
  if grep -q 'login=3' "$RUNNER_TEMP/success-gateway.log" && [ "$(grep -c 'handshake=6' "$RUNNER_TEMP/success-gateway.log" || true)" -ge 4 ]; then break; fi
  sleep 1
done
if ! grep -q 'login=3' "$RUNNER_TEMP/success-gateway.log" || [ "$(grep -c 'handshake=6' "$RUNNER_TEMP/success-gateway.log" || true)" -lt 4 ]; then
  cat "$RUNNER_TEMP/success-gateway.log"
  adb logcat -d -s VPN:I GoLog:I AndroidRuntime:E
  echo 'VPN did not reconnect after disconnect' >&2
  exit 1
fi
if ! adb shell /data/local/tmp/android-udp-probe; then
  cat "$RUNNER_TEMP/success-gateway.log"
  echo 'UDP failed after disconnect and reconnect' >&2
  exit 1
fi
echo 'UDP recovered after disconnect and reconnect'
if ! adb shell /data/local/tmp/android-tcp-probe; then
  echo 'TCP failed after disconnect and reconnect' >&2
  exit 1
fi
tap_field btnToggle
