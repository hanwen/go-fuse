package main

// Given a file with filenames, this times how fast we can stat them
// in parallel.  This is useful for benchmarking purposes.

import (
	"bufio"
	"flag"
	"fmt"
	"github.com/hanwen/go-fuse/fuse"
	"math"
	"os"
	"runtime"
	"sort"
	"time"
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

	totalRuns := *runs + 1

	results := make([]float64, 0)
	for j := 0; j < totalRuns; j++ {
		result := BulkStat(*threads, files)
		if j > 0 {
			results = append(results, result)
		} else {
			fmt.Println("Ignoring first run to preheat caches.")
		}

		if j < totalRuns-1 {
			fmt.Printf("Sleeping %.2f seconds\n", *sleepTime)
			time.Sleep(int64(*sleepTime * 1e9))
		}
	}

	Analyze(results)
}

func Analyze(times []float64) {
	sorted := times
	sort.SortFloat64s(sorted)

	tot := 0.0
	for _, v := range times {
		tot += v
	}
	n := float64(len(times))

	avg := tot / n
	variance := 0.0
	for _, v := range times {
		variance += (v - avg)*(v - avg)
	}
	variance /= n

	stddev := math.Sqrt(variance)

	median := sorted[len(times)/2]
	perc90 := sorted[int(n * 0.9)]
	perc10 := sorted[int(n * 0.1)]

	fmt.Printf(
		"%d samples\n" +
		"avg %.2f ms 2sigma %.2f " +
		"median %.2fms\n"  +
		"10%%tile %.2fms, 90%%tile %.2fms\n",
		len(times), avg, 2*stddev, median, perc10, perc90)
}

// Returns milliseconds.
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
