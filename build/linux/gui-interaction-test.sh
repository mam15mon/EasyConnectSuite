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
    exit 0
  fi
  sleep .1
done
echo 'GUI profile was not saved with the expected values' >&2
exit 1
