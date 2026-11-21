#!/usr/bin/env python3
"""Write a partition via MTK preloader SEND_PARTITION_DATA (0x70).

usage: part_write.py <partition> <image-file> [--check]

Performs the handshake, echoes 0x70, sends partition name (64 bytes),
length (4 bytes BE), reads status, sends data (0x200 chunks), sends checksum.
"""
import os, sys, time, glob, termios, select, datetime, struct

LOG = open('/tmp/part_write.log', 'ab', buffering=0)
def log(s):
    LOG.write((datetime.datetime.now().strftime('%H:%M:%S.%f')[:-3] + ' ' + s + '\n').encode())

def raw(fd):
    a = termios.tcgetattr(fd)
    a[0] = 0; a[1] = 0
    a[2] = termios.CLOCAL | termios.CREAD | termios.CS8
    a[3] = 0
    a[4] = termios.B115200; a[5] = termios.B115200
    a[6][termios.VMIN] = 0; a[6][termios.VTIME] = 0
    termios.tcsetattr(fd, termios.TCSANOW, a)

def rd(fd, t=0.5, n=4096):
    end = time.time() + t
    out = b''
    while time.time() < end:
        r, _, _ = select.select([fd], [], [], max(0, end - time.time()))
        if not r:
            break
        try:
            d = os.read(fd, n)
        except OSError:
            break
        if not d:
            break
        out += d
    return out

def wr_all(fd, data, timeout=10.0):
    end = time.time() + timeout
    mv = memoryview(data)
    pos = 0
    while pos < len(data) and time.time() < end:
        try:
            n = os.write(fd, mv[pos:])
            if n > 0:
                pos += n
        except BlockingIOError:
            select.select([], [fd], [], 0.05)
        except OSError as e:
            log("write error: " + str(e))
            return False
    return pos == len(data)

START = bytes([0xA0, 0x0A, 0x50, 0x05])
EXP = bytes([(~b) & 0xFF for b in START])

def handshake(fd, timeout=4.0):
    got = 0
    t0 = time.time()
    while time.time() - t0 < timeout:
        if not wr_all(fd, START[got:got + 1]):
            return False
        d = rd(fd, 0.06)
        for b in d:
            if b == EXP[got]:
                got += 1
                if got == 4:
                    termios.tcflush(fd, termios.TCIOFLUSH)
                    return True
            else:
                got = 1 if b == START[0] else 0
    return False

def echo_cmd(fd, cmd, t=1.0):
    wr_all(fd, bytes([cmd]))
    d = b''
    end = time.time() + t
    while time.time() < end:
        d += rd(fd, 0.1)
        if d:
            break
    return d

def xflash_sum(data):
    s = 0
    pos = 0
    for i in range(len(data) // 4):
        s = (s + struct.unpack('<I', data[i * 4:i * 4 + 4])[0]) & 0xFFFFFFFF
        pos += 4
    if len(data) % 4:
        for i in range(4 - (len(data) % 4)):
            s = (s + data[pos + i]) & 0xFFFFFFFF
    return s

def main():
    part = sys.argv[1]
    path = sys.argv[2]
    data = open(path, 'rb').read()
    log(f"== write {part} from {path} size={len(data)}")

    # wait for device
    fd = None
    deadline = time.time() + 300
    while time.time() < deadline:
        ports = [p for p in sorted(glob.glob('/dev/ttyACM*'))
                 if os.path.exists('/sys/class/tty/' + os.path.basename(p))]
        if not ports:
            time.sleep(0.02)
            continue
        try:
            fd = os.open(ports[0], os.O_RDWR | os.O_NOCTTY | os.O_NONBLOCK)
        except OSError:
            time.sleep(0.02)
            continue
        raw(fd)
        if handshake(fd):
            log("handshake OK on " + ports[0])
            break
        try: os.close(fd)
        except Exception: pass
        fd = None
    if fd is None:
        log("no device/handshake")
        return 2

    time.sleep(0.05)
    d = echo_cmd(fd, 0x70)
    log("echo 0x70 -> " + d.hex())
    if not d or d[0] != 0x70:
        log("ABORT: 0x70 not echoed")
        return 3

    name = part.encode()[:64]
    name = name + b'\x00' * (64 - len(name))
    if not wr_all(fd, name):
        log("ABORT: name write failed"); return 4
    if not wr_all(fd, struct.pack('>I', len(data))):
        log("ABORT: len write failed"); return 5
    st = rd(fd, 1.0, 8)
    log("status after header: " + st.hex())
    if not st:
        log("ABORT: no status"); return 6

    # send data in 0x200 chunks
    pos = 0
    t0 = time.time()
    while pos < len(data):
        chunk = data[pos:pos + 0x200]
        if not wr_all(fd, chunk, timeout=5.0):
            log(f"ABORT: data write failed at {pos}"); return 7
        pos += len(chunk)
        if pos % (0x200 * 64) == 0:
            log(f"  sent {pos}/{len(data)} ({time.time()-t0:.1f}s)")
    log("data sent in %.1fs" % (time.time() - t0))
    wr_all(fd, struct.pack('>I', xflash_sum(data)))
    st2 = rd(fd, 2.0, 16)
    log("final status: " + st2.hex())
    try: os.close(fd)
    except Exception: pass
    log("done")
    return 0

if __name__ == '__main__':
    sys.exit(main())
