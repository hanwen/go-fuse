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
	threads := flag.Int("threads", 12, "number of parallel threads in a run.")
	sleepTime := flag.Float64("sleep", 4.0, "amount of sleep between runs.")
	runs := flag.Int("runs", 10, "number of runs.")

	flag.Parse()

	filename := flag.Args()[0]
	f, err := os.Open(filename)
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

	tot := 0.0
	totalRuns := *runs + 1

	for j := 0 ; j < totalRuns; j++ {
		result := BulkStat(*threads, files)
		if j > 0 {
			tot += result
		} else {
			fmt.Println("Ignoring first run to preheat caches.")
		}
		if j < totalRuns-1 {
			fmt.Printf("Sleeping %.2f seconds\n", *sleepTime)
			time.Sleep(int64(*sleepTime * 1e9))
		}
	}

	fmt.Printf("Average of %d runs: %f ms\n", *runs, tot/float64(*runs))
}

func BulkStat(parallelism int, files []string) float64 {
	todo := make(chan string, len(files))
	dts := make(chan int64, parallelism)

	allStart := time.Nanoseconds()

	fmt.Printf("Statting %d files with %d threads\n", len(files), parallelism)
	for i := 0; i < parallelism; i++ {
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
	avg := total / float64(len(files))

	fmt.Printf("Elapsed: %f sec. Average stat %f ms\n",
		float64(allEnd-allStart)*1e-9, avg)

	return avg
}
