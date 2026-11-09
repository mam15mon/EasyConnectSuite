#!/usr/bin/env bash
set -euo pipefail

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
    connection, _ = server.accept()
    with connection:
        connection.settimeout(30)
        open(sys.argv[1], "w").close()
        while connection.recv(4096):
            pass
        open(sys.argv[2], "w").close()
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
  if [ -f "$RUNNER_TEMP/mock-accepted" ]; then break; fi
  sleep 1
done
if [ ! -f "$RUNNER_TEMP/mock-accepted" ]; then
  echo "Mock gateway did not receive the connection" >&2
  exit 1
fi
adb shell input swipe 500 1500 500 400 400
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
  if [ -f "$RUNNER_TEMP/mock-closed" ]; then break; fi
  sleep 1
done
if [ ! -f "$RUNNER_TEMP/mock-closed" ]; then
  echo "Connection remained open after cancellation" >&2
  exit 1
fi
printf '%s\n' 'Mock gateway observed the authentication connection close'
