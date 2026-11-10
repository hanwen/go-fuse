#!/usr/bin/env python3
# Drives OpenBSD over the serial console.
#
# install: at the VGA boot> prompt of the install CD, switch the
# console to com0 by typing into the QEMU monitor; then go to the
# installer shell, write the response file and run an unattended
# install.
#
# config: log in to the installed system as root, install rc.local
# and power off.
import os, re, socket, subprocess, sys, threading, time

logname, mode, conf, monsock = sys.argv[1:5]
qemu = sys.argv[5:] + ["-monitor", "unix:%s,server,nowait" % monsock]
log = open(logname, "ab")
p = subprocess.Popen(qemu, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
buf = bytearray()
cond = threading.Condition()

def reader():
    while True:
        b = os.read(p.stdout.fileno(), 4096)
        if not b:
            break
        log.write(b); log.flush()
        with cond:
            buf.extend(b)
            cond.notify_all()
    with cond:
        cond.notify_all()

threading.Thread(target=reader, daemon=True).start()
pos = 0

def fail(msg):
    p.kill()
    sys.exit("openbsd driver: " + msg)

def expect(pat, timeout):
    global pos
    r = re.compile(pat)
    deadline = time.time() + timeout
    with cond:
        while True:
            m = r.search(buf, pos)
            if m:
                pos = m.end()
                return
            if p.poll() is not None:
                fail("qemu exited waiting for %r" % pat)
            if time.time() > deadline:
                fail("timed out waiting for %r" % pat)
            cond.wait(1)

def send(s):
    for c in s.encode():
        p.stdin.write(bytes([c])); p.stdin.flush()
        time.sleep(0.01)

def halt():
    # The install kernel halts without powering off, so stop QEMU
    # once the disks are synced.
    send("halt -p\n")
    r = re.compile(rb"syncing disks\.\.\. done")
    deadline = time.time() + 120
    with cond:
        while p.poll() is None and not r.search(buf, pos):
            if time.time() > deadline:
                fail("guest did not halt")
            cond.wait(1)
    try:
        p.wait(30)
    except subprocess.TimeoutExpired:
        p.kill()
        p.wait()

def monitor():
    for _ in range(100):
        try:
            m = socket.socket(socket.AF_UNIX)
            m.connect(monsock)
            m.settimeout(5)
            return m
        except OSError:
            time.sleep(0.1)
    fail("cannot connect to QEMU monitor")

def hmp(m, cmd):
    m.sendall((cmd + "\n").encode())
    out = b""
    while not out.endswith(b"(qemu) "):
        out += m.recv(65536)
    return out.decode(errors="replace")

def vga_text(m):
    out = hmp(m, "xp /2000xh 0xb8000")
    return "".join(chr(int(w, 16) & 0xff) for w in re.findall(r"0x([0-9a-f]{4})\b", out))

def sendkeys(m, s):
    names = {" ": "spc", "\n": "ret"}
    for c in s:
        hmp(m, "sendkey " + names.get(c, c))
        time.sleep(0.05)

if mode == "install":
    m = monitor()
    hmp(m, "")
    deadline = time.time() + 120
    while "boot>" not in vga_text(m):
        if p.poll() is not None or time.time() > deadline:
            fail("no boot> prompt on VGA")
        time.sleep(0.2)
    sendkeys(m, "set tty com0\n")
    time.sleep(1)
    send("boot\n")
    expect(rb"\(I\)nstall, \(U\)pgrade, \(A\)utoinstall or \(S\)hell\? ", 600)
    send("s\n")
    expect(rb"# ", 60)
    send("cat > /auto_install.conf <<'EOF'\n" + open(conf).read() + "EOF\n")
    send("/install -af /auto_install.conf\n")
    expect(rb"CONGRATULATIONS", 3600)
    expect(rb"# ", 60)
    halt()
elif mode == "config":
    expect(rb"login: ", 600)
    send("root\n")
    expect(rb"Password:", 60)
    send("gofuse\n")
    expect(rb"# ", 60)
    send("cat > /etc/rc.local <<'EOF'\n" + open(conf).read() + "EOF\n")
    send("echo gofuse-config-$((40+2))\n")
    expect(rb"gofuse-config-42", 60)
    halt()
else:
    fail("unknown mode " + mode)
