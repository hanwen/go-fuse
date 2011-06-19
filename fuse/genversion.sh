#!/bin/sh

VERSION=\"$(git log -n1 --pretty=format:'%h (%cd)' --date=iso )\"

cat <<EOF
package fuse
func init() {
	version = new(string)
	*version = ${VERSION}
}
EOF
