#!/usr/bin/python3
"""Wait for the visible desktop button to become actionable via AT-SPI."""
import sys
import time

import pyatspi


def actionable(node, name):
    try:
        state = node.getState()
        if (node.getRole() == pyatspi.ROLE_PUSH_BUTTON and node.name == name
                and state.contains(pyatspi.STATE_SHOWING)
                and state.contains(pyatspi.STATE_SENSITIVE)
                and state.contains(pyatspi.STATE_ENABLED)):
            return True
        return any(actionable(child, name) for child in node)
    except (RuntimeError, LookupError):
        return False


label = sys.argv[1]
deadline = time.monotonic() + 15
while time.monotonic() < deadline:
    if actionable(pyatspi.Registry.getDesktop(0), label):
        print(f'GUI button {label} is actionable', flush=True)
        break
    time.sleep(.1)
else:
    raise SystemExit(f'GUI button {label} did not become actionable')
