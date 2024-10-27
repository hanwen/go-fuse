// Copyright 2024 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vhostuser

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func mkinitRam(path string, busybox string, initContent []byte) error {
	dirs := []string{"etc", "sys", "proc", "mnt",
		"bin", "sbin", "usr", "usr/sbin", "usr/bin"}

	namesByDir := map[string][]string{
		"bin": strings.Split(`arch
ash
base32
base64
busybox
cat
chattr
chgrp
chmod
chown
conspy
cp
cpio
cttyhack
date
dd
df
dmesg
dnsdomainname
dumpkmap
echo
ed
egrep
false
fatattr
fdflush
fgrep
fsync
getopt
grep
gunzip
gzip
hostname
hush
ionice
iostat
ipcalc
kbd_mode
kill
link
linux32
linux64
ln
login
ls
lsattr
lzop
makemime
mkdir
mknod
mktemp
more
mount
mountpoint
mpstat
mt
mv
netstat
nice
pidof
ping
ping6
pipe_progress
printenv
ps
pwd
reformime
resume
rev
rm
rmdir
rpm
run-parts
scriptreplay
sed
setarch
setpriv
setserial
sh
sleep
stat
stty
su
sync
tar
touch
true
umount
uname
usleep
vi
watch
zcat`, "\n"),

		"sbin": strings.Split(`acpid
adjtimex
arp
blkid
blockdev
bootchartd
depmod
devmem
fbsplash
fdisk
findfs
freeramdisk
fsck
fsck.minix
fstrim
getty
halt
hdparm
hwclock
ifconfig
ifdown
ifenslave
ifup
init
insmod
ip
ipaddr
iplink
ipneigh
iproute
iprule
iptunnel
klogd
loadkmap
logread
losetup
lsmod
makedevs
mdev
mkdosfs
mke2fs
mkfs.ext2
mkfs.minix
mkfs.vfat
mkswap
modinfo
modprobe
nameif
pivot_root
poweroff
raidautorun
reboot
rmmod
route
run-init
runlevel
setconsole
slattach
start-stop-daemon
sulogin
swapoff
swapon
switch_root
sysctl
syslogd
tc
tunctl
udhcpc
uevent
vconfig
watchdog
zcip`, "\n"),
		"usr/bin": strings.Split(`[
[[
ascii
awk
basename
bc
beep
blkdiscard
bunzip2
bzcat
bzip2
cal
chpst
chrt
chvt
cksum
clear
cmp
comm
crc32
crontab
cryptpw
cut
dc
deallocvt
diff
dirname
dos2unix
dpkg
dpkg-deb
du
dumpleases
eject
env
envdir
envuidgid
expand
expr
factor
fallocate
fgconsole
find
flock
fold
free
ftpget
ftpput
fuser
groups
hd
head
hexdump
hexedit
hostid
id
install
ipcrm
ipcs
killall
last
less
logger
logname
lpq
lpr
lsof
lspci
lsscsi
lsusb
lzcat
lzma
man
md5sum
mesg
microcom
mkfifo
mkpasswd
nc
nl
nmeter
nohup
nproc
nsenter
nslookup
od
openvt
passwd
paste
patch
pgrep
pkill
pmap
printf
pscan
pstree
pwdx
readlink
realpath
renice
reset
resize
rpm2cpio
runsv
runsvdir
rx
script
seq
setfattr
setkeycodes
setsid
setuidgid
sha1sum
sha256sum
sha3sum
sha512sum
showkey
shred
shuf
smemcap
softlimit
sort
split
ssl_client
strings
sum
sv
svc
svok
tac
tail
taskset
tcpsvd
tee
telnet
test
tftp
time
timeout
top
tr
traceroute
traceroute6
tree
truncate
ts
tsort
tty
ttysize
udhcpc6
udpsvd
unexpand
uniq
unix2dos
unlink
unlzma
unshare
unxz
unzip
uptime
users
uudecode
uuencode
vlock
volname
w
wall
wc
wget
which
who
whoami
whois
xargs
xxd
xz
xzcat
yes`, "\n"),
		"usr/sbin": strings.Split(`addgroup
add-shell
adduser
arping
brctl
chat
chpasswd
chroot
crond
delgroup
deluser
dhcprelay
dnsd
ether-wake
fakeidentd
fbset
fdformat
fsfreeze
ftpd
httpd
i2cdetect
i2cdump
i2cget
i2cset
i2ctransfer
ifplugd
inetd
killall5
loadfont
lpd
mim
nanddump
nandwrite
nbd-client
nologin
ntpd
partprobe
popmaildir
powertop
rdate
rdev
readahead
readprofile
remove-shell
rtcwake
seedrng
sendmail
setfont
setlogcons
svlogd
telnetd
tftpd
ubiattach
ubidetach
ubimkvol
ubirename
ubirmvol
ubirsvol
ubiupdatevol
udhcpd`, "\n"),
	}

	dir, err := os.MkdirTemp("", "")
	if err != nil {
		return err
	}

	data, err := os.ReadFile(busybox)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "busybox"), data, 0755); err != nil {
		return err
	}
	files := []string{"busybox"}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(dir, d), 0755); err != nil {
			return err
		}

		files = append(files, d)
		for _, nm := range namesByDir[d] {
			if nm == "" {
				continue
			}
			files = append(files, filepath.Join(d, nm))
			target := strings.Repeat("../", 1+strings.Count(d, "/")) + "busybox"
			if err := os.Symlink(target, filepath.Join(dir, d, nm)); err != nil {
				return err
			}
		}
	}

	if err := os.Symlink("busybox", filepath.Join(dir, "linuxrc")); err != nil {
		return err
	}
	files = append(files, "linuxrc")
	if initContent != nil {
		if err := os.WriteFile(filepath.Join(dir, "init"), initContent, 0755); err != nil {
			return err
		}
	} else {
		if err := os.Symlink("busybox", filepath.Join(dir, "init")); err != nil {
			return err
		}
	}
	files = append(files, "init")

	cmd := exec.Command("cpio", "--null", "-ov", "--format=newc")
	cmd.Stdin = bytes.NewBufferString(strings.Join(files, "\000") + "\000")

	out := &bytes.Buffer{}
	cmd.Stderr = out
	cmd.Stdout = out
	cmd.Dir = dir
	f, err := os.Create(path)
	if err != nil {
		return err
	}

	gzw := gzip.NewWriter(f)
	cmd.Stdout = gzw
	if err := cmd.Run(); err != nil {
		return err
	}
	if err := gzw.Close(); err != nil {
		return fmt.Errorf("cmd %v: %v, out %s", cmd.Args, err, out.String())
	}
	return nil
}
