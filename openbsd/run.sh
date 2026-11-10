#!/bin/sh
# Runs the go-fuse tests on the test CD, as root from rc.local; see
# test.bash. The CD has the test binaries in bin/, their names in
# tests.txt and the flags for them in args.txt.
exec > /dev/console 2>&1
CD=/mnt/gofuse
RES=/root/results
mkdir -p $RES /root/gofuse
cp $CD/bin/*.test $CD/tests.txt $CD/args.txt /root/gofuse/
umount $CD
chmod +x /root/gofuse/*.test
uname -a > $RES/uname.txt

# Each /dev/fuseN serves one mount; tests may mount several at once.
cd /dev
sh MAKEDEV sd1 fuse >/dev/null 2>&1
maj=$(stat -f %Hr fuse0)
i=1
while [ $i -lt 8 ]; do
	[ -e fuse$i ] || mknod -m 600 fuse$i c $maj $i
	i=$((i + 1))
done

cd /root/gofuse
read -r args < args.txt
set -f
for t in $(cat tests.txt); do
	echo "go-fuse: running $t"
	./$t.test -test.v -test.timeout 10m $args > $RES/$t.log 2>&1
	echo $? > $RES/$t.status
	echo "go-fuse: $t exited $(cat $RES/$t.status)"
done
set +f

mount > $RES/mounts.txt
echo done > $RES/finished
tar -cf /dev/rsd1c -C $RES .
echo "go-fuse: results written; powering off"
sync
halt -p
