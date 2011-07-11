package main

// Given a file with filenames, this times how fast we can stat them
// in parallel.  This is useful for benchmarking purposes.

import (
	"bufio"
	"flag"
	"github.com/hanwen/go-fuse/benchmark"
	"os"
	"runtime"
)

func main() {
	threads := flag.Int("threads", 0, "number of parallel threads in a run. If 0, use CPU count.")
	sleepTime := flag.Float64("sleep", 4.0, "amount of sleep between runs.")
	runs := flag.Int("runs", 10, "number of runs.")

	flag.Parse()
	if *threads == 0 {
		*threads = fuse.CountCpus()
		runtime.GOMAXPROCS(*threads)
	}
	filename := flag.Args()[0]
	f, err := os.Open(filename)
	if err != nil {
		panic("err" + err.String())
	}

	reader := bufio.NewReader(f)

	files := make([]string, 0)
	for {
		l, _, err := reader.ReadLine()
		if err != nil {
			break
		}
		files = append(files, string(l))
	}

	results := fuse.RunBulkStat(*runs, *threads, *sleepTime, files)
	fuse.AnalyzeBenchmarkRuns(results)
}
