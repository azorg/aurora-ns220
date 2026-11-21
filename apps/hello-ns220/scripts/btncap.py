#!/usr/bin/env python3
# btncap.py — логгер нажатий аппаратных кнопок (evdev) для NS220/Aurora.
# Запуск: python3 -u btncap.py > /home/user/btncap.log 2>&1 &
import os, select, struct

KEYS = {114: "VOLUMEDOWN", 115: "VOLUMEUP", 116: "POWER", 102: "HOME", 158: "BACK",
        139: "MENU", 212: "CAMERA", 103: "UP", 108: "DOWN", 105: "LEFT", 106: "RIGHT",
        28: "ENTER", 1: "ESC", 143: "WAKEUP", 152: "POWER2", 113: "MUTE", 217: "SEARCH",
        226: "MEDIA", 172: "HOMEPAGE"}
SW = {0: "LID", 1: "TABLET_MODE", 2: "HEADPHONE_INSERT", 3: "RADIO", 4: "MICROPHONE_INSERT"}


def devnames():
    m = {}
    try:
        cur = None
        for line in open("/proc/bus/input/devices"):
            line = line.strip()
            if line.startswith("N: Name="):
                cur = line.split("=", 1)[1].strip('"')
            elif line.startswith("H: Handlers=") and cur:
                for h in line.split("=", 1)[1].split():
                    if h.startswith("event"):
                        m["/dev/input/" + h] = cur
    except Exception:
        pass
    return m


names = devnames()
fds = {}
for i in range(0, 8):
    p = "/dev/input/event%d" % i
    try:
        fds[os.open(p, os.O_RDONLY | os.O_NONBLOCK)] = p
    except OSError:
        pass

print("READY", flush=True)
for fd, p in fds.items():
    print("open %s (%s)" % (p, names.get(p, "?")), flush=True)

while True:
    try:
        r, _, _ = select.select(list(fds), [], [], 1.0)
    except InterruptedError:
        continue
    for fd in r:
        try:
            data = os.read(fd, 16 * 64)
        except OSError:
            continue
        for off in range(0, len(data) - 15, 16):
            sec, usec, typ, code, val = struct.unpack("<iiHHi", data[off:off + 16])
            if typ == 1:
                n = KEYS.get(code, "KEY_%d" % code)
                print("%.6f %-13s %-14s %-12s %s" % (
                    sec + usec / 1e6, p.split("/")[-1], names.get(p, "?"), n,
                    "PRESS" if val else "release"), flush=True)
            elif typ == 5:
                print("%.6f %-13s %-14s SW_%s %s" % (
                    sec + usec / 1e6, p.split("/")[-1], names.get(p, "?"),
                    SW.get(code, code), val), flush=True)
