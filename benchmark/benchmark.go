package benchmark

// Routines for benchmarking fuse.

import (
	"bufio"
	"log"
	"os"
)

func ReadLines(name string) []string {
	f, err := os.Open(name)
	if err != nil {
		log.Fatal("ReadLines: ", err)
	}
	defer f.Close()
	r := bufio.NewReader(f)

	l := []string{}
	for {
		line, _, err := r.ReadLine()
		if line == nil || err != nil {
			break
		}

		fn := string(line)
		l = append(l, fn)
	}
	if len(l) == 0 {
		log.Fatal("no files added")
	}

	return l
}
