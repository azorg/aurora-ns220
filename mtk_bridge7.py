import os, time, glob, termios, select, pty, sys, datetime

LOG = open('/tmp/bridge.log', 'ab', buffering=0)
def log(s):
    LOG.write((datetime.datetime.now().strftime('%H:%M:%S.%f')[:-3] + ' ' + s + '\n').encode())

master, slave = pty.openpty()
slave_name = os.ttyname(slave)
link = '/tmp/ttyMTK'
try: os.unlink(link)
except OSError: pass
os.symlink(slave_name, link)
log(f"pty {slave_name} -> {link}")

def raw(fd):
    a = termios.tcgetattr(fd)
    a[0] = 0; a[1] = 0
    a[2] = termios.CLOCAL | termios.CREAD | termios.CS8
    a[3] = 0
    a[4] = termios.B115200; a[5] = termios.B115200
    a[6][termios.VMIN] = 0; a[6][termios.VTIME] = 0
    termios.tcsetattr(fd, termios.TCSANOW, a)

def set_nonblock(fd):
    fl = os.get_blocking(fd)
    os.set_blocking(fd, False)
    return fl

raw(master)
set_nonblock(master)

START = bytes([0xA0, 0x0A, 0x50, 0x05])
EXP = bytes([(~b) & 0xFF for b in START])

def do_handshake(fd):
    got = 0
    t0 = time.time()
    while time.time() - t0 < 4.0:
        try:
            os.write(fd, START[got:got + 1])
        except OSError:
            return False
        r, _, _ = select.select([fd], [], [], 0.06)
        if not r:
            continue
        try:
            d = os.read(fd, 256)
        except OSError:
            return False
        if not d:
            continue
        log("hs rx " + d.hex()[:40])
        for b in d:
            if b == EXP[got]:
                got += 1
                if got == 4:
                    try: termios.tcflush(fd, termios.TCIOFLUSH)
                    except Exception: pass
                    return True
            else:
                got = 1 if b == START[0] else 0
    return False

def find_port():
    ports = [p for p in sorted(glob.glob('/dev/ttyACM*'))
             if os.path.exists('/sys/class/tty/' + os.path.basename(p))]
    return ports[0] if ports else None

acm = None
acmpath = None
opened_at = 0.0
to_acm = bytearray()
to_mtk = bytearray()
deadline = time.time() + float(sys.argv[1] if len(sys.argv) > 1 else 1200)
dropped = 0

def pump():
    global acm, acmpath, dropped
    # host -> device
    if acm is not None and to_acm:
        try:
            n = os.write(acm, bytes(to_acm))
            if n > 0:
                del to_acm[:n]
        except BlockingIOError:
            pass
        except OSError:
            log("ACM write error")
            try: os.close(acm)
            except Exception: pass
            acm = None; acmpath = None
            to_acm.clear()
    # device -> host
    if to_mtk:
        try:
            n = os.write(master, bytes(to_mtk))
            if n > 0:
                del to_mtk[:n]
        except BlockingIOError:
            pass
        except OSError:
            to_mtk.clear()

while time.time() < deadline:
    if acm is not None:
        if not os.path.exists('/sys/class/tty/' + os.path.basename(acmpath)) and (time.time() - opened_at) > 2.0:
            log("ACM gone")
            try: os.close(acm)
            except Exception: pass
            acm = None; acmpath = None
            to_acm.clear()
    if acm is None:
        p = find_port()
        if p is None:
            time.sleep(0.01)
            continue
        try:
            fd = os.open(p, os.O_RDWR | os.O_NOCTTY | os.O_NONBLOCK)
        except OSError:
            time.sleep(0.01)
            continue
        try:
            raw(fd)
        except Exception as e:
            log("raw err " + str(e))
        try:
            set_nonblock(fd)
        except Exception:
            pass
        log("ACM opened " + p)
        acm, acmpath, opened_at = fd, p, time.time()
        if not do_handshake(fd):
            log("BRIDGE HS FAIL")
            try: os.close(fd)
            except Exception: pass
            acm = None; acmpath = None
            continue
        log("BRIDGE HS OK")

    pump()

    rlist = [master]
    wlist = []
    if acm is not None:
        rlist.append(acm)
        if to_acm:
            wlist.append(acm)
    if to_mtk:
        wlist.append(master)
    try:
        r, w, _ = select.select(rlist, wlist, [], 0.01)
    except Exception:
        r, w = [], []
    if w:
        pump()
    for fd in r:
        if fd == master:
            try:
                data = os.read(master, 65536)
            except OSError:
                data = b''
            if data:
                if acm is not None:
                    to_acm.extend(data)
                else:
                    dropped += len(data)
        elif fd == acm:
            try:
                data = os.read(acm, 65536)
            except OSError:
                data = b''
            if not data:
                log("ACM EOF")
                try: os.close(acm)
                except Exception: pass
                acm = None; acmpath = None
                continue
            to_mtk.extend(data)
    pump()
log(f"bridge exit, dropped_host_bytes={dropped}")
