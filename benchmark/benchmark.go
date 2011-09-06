package fuse

// Routines for benchmarking fuse.

import (
	"fmt"
	"log"
	"math"
	"os"
	"regexp"
	"sort"
	"time"
)

// Used for benchmarking.  Returns milliseconds.
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
				_, err := os.Lstat(fn)
				if err != nil {
					log.Fatal("All stats should succeed:", err)
				}
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

func AnalyzeBenchmarkRuns(times []float64) {
	sorted := times
	sort.Float64s(sorted)

	tot := 0.0
	for _, v := range times {
		tot += v
	}
	n := float64(len(times))

	avg := tot / n
	variance := 0.0
	for _, v := range times {
		variance += (v - avg) * (v - avg)
	}
	variance /= n

	stddev := math.Sqrt(variance)

	median := sorted[len(times)/2]
	perc90 := sorted[int(n*0.9)]
	perc10 := sorted[int(n*0.1)]

	fmt.Printf(
		"%d samples\n"+
			"avg %.3f ms 2sigma %.3f "+
			"median %.3fms\n"+
			"10%%tile %.3fms, 90%%tile %.3fms\n",
		len(times), avg, 2*stddev, median, perc10, perc90)
}

func RunBulkStat(runs int, threads int, sleepTime float64, files []string) (results []float64) {
	runs++
	for j := 0; j < runs; j++ {
		result := BulkStat(threads, files)
		if j > 0 {
			results = append(results, result)
		} else {
			fmt.Println("Ignoring first run to preheat caches.")
		}

		if j < runs-1 {
			fmt.Printf("Sleeping %.2f seconds\n", sleepTime)
			time.Sleep(int64(sleepTime * 1e9))
		}
	}
	return results
}

func CountCpus() int {
	var contents [10240]byte

	f, err := os.Open("/proc/stat")
	defer f.Close()
	if err != nil {
		return 1
	}
	n, _ := f.Read(contents[:])
	re, _ := regexp.Compile("\ncpu[0-9]")

	return len(re.FindAllString(string(contents[:n]), 100))
}
