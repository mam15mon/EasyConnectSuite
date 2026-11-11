#!/usr/bin/python3
"""Select proxy-only mode through the real WebKit accessibility controls."""
import time

import pyatspi


def find_control(node, name):
    try:
        state = node.getState()
        if (node.name == name and state.contains(pyatspi.STATE_SHOWING)
                and state.contains(pyatspi.STATE_SENSITIVE)
                and state.contains(pyatspi.STATE_ENABLED)
                and node.queryAction().nActions > 0):
            return node
    except (RuntimeError, LookupError, NotImplementedError):
        pass
    for child in node:
        found = find_control(child, name)
        if found is not None:
            return found
    return None


def wait_control(name):
    deadline = time.monotonic() + 15
    while time.monotonic() < deadline:
        node = find_control(pyatspi.Registry.getDesktop(0), name)
        if node is not None:
            return node
        time.sleep(.1)
    raise SystemExit(f'GUI control {name} did not become actionable')


if not wait_control('连接设置').queryAction().doAction(0):
    raise SystemExit('Could not open GUI connection settings')
for name, checked in [('TUN 分流', False), ('本地代理', True)]:
    node = wait_control(name)
    if node.getState().contains(pyatspi.STATE_CHECKED) == checked:
        raise SystemExit(f'Unexpected initial GUI state for {name}')
    if not node.queryAction().doAction(0):
        raise SystemExit(f'Could not toggle {name}')
    deadline = time.monotonic() + 15
    while time.monotonic() < deadline:
        node = wait_control(name)
        if node.getState().contains(pyatspi.STATE_CHECKED) == checked:
            break
        time.sleep(.1)
    else:
        raise SystemExit(f'GUI state did not change for {name}')
print('GUI selected proxy-only mode through settings controls', flush=True)
