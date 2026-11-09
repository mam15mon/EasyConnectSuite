#!/usr/bin/env python3
"""Expose a real Linux TCP echo endpoint to the synthetic VPN gateway."""
import fcntl
import os
import select
import socket
import struct
import subprocess
import threading

TUNSETIFF = 0x400454CA
IFF_TUN = 0x0001
IFF_NO_PI = 0x1000

tun = os.open('/dev/net/tun', os.O_RDWR)
fcntl.ioctl(tun, TUNSETIFF, struct.pack('16sH', b'ecs-test', IFF_TUN | IFF_NO_PI))
subprocess.run(['ip', 'addr', 'add', '10.90.0.3', 'peer', '10.90.0.2', 'dev', 'ecs-test'], check=True)
subprocess.run(['ip', 'link', 'set', 'ecs-test', 'up'], check=True)

bridge = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
bridge.bind(('127.0.0.1', 18446))
listener = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
listener.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
listener.bind(('10.90.0.3', 12345))
listener.listen()

def echo():
    while True:
        conn, _ = listener.accept()
        with conn:
            while True:
                data = conn.recv(4096)
                if not data:
                    break
                conn.sendall(data)

threading.Thread(target=echo, daemon=True).start()
peer = None
print('tcp_gateway_ready', flush=True)
while True:
    ready, _, _ = select.select([tun, bridge], [], [])
    if bridge in ready:
        packet, peer = bridge.recvfrom(2048)
        os.write(tun, packet)
    if tun in ready:
        packet = os.read(tun, 2048)
        if peer is not None:
            bridge.sendto(packet, peer)
