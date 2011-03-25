package main

// Given a file with filenames, this times how fast we can stat them
// in parallel.  This is useful for benchmarking purposes.

import (
	"os"
	"flag"
	"time"
	"fmt"
	"encoding/line"
)

func main() {
	flag.Parse()

	filename := flag.Args()[0]
	f, err := os.Open(filename, os.O_RDONLY, 0)
	if err != nil {
		panic("err" + err.String())
	}

	linelen := 1000
	reader := line.NewReader(f, linelen)

	files := make([]string, 0)
	for {
		l, _, err := reader.ReadLine()
		if err != nil {
			break
		}
		files = append(files, string(l))
	}

	runs := 10
	tot := 0.0
	sleeptime := 4.0
	for j := 0; j < runs; j++ {
		tot += BulkStat(10, files)
		fmt.Printf("Sleeping %.2f seconds\n", sleeptime)
		time.Sleep(int64(sleeptime * 1e9))
	}

	fmt.Printf("Average of %d runs: %f ms\n", runs, tot/float64(runs))
}

func BulkStat(parallelism int, files []string) float64 {
	parallel := 10
	todo := make(chan string, len(files))
	dts := make(chan int64, parallel)

	allStart := time.Nanoseconds()

	fmt.Printf("Statting %d files with %d threads\n", len(files), parallel)
	for i := 0; i < parallel; i++ {
		go func() {
			for {
				fn := <-todo
				if fn == "" {
					break
				}

				t := time.Nanoseconds()
				os.Lstat(fn)
				dts <- time.Nanoseconds() - t
			}
		}()
	}

	for _, v := range files {
		todo <- v
	}

	total := 0.0
	for i := 0; i < len(files); i++ {
		total += float64(<-dts) * 1e-6
	}

	allEnd := time.Nanoseconds()
	avg := total/float64(len(files))

	fmt.Printf("Elapsed: %f sec. Average stat %f ms\n",
		float64(allEnd-allStart)*1e-9, avg)

	return avg
}
