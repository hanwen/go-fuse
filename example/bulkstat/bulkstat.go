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
		panic("err"+err.String())
	}

	linelen := 1000
	reader := line.NewReader(f, linelen)

	files := make([]string, 0)
	for {
		l, _, err :=  reader.ReadLine()
		if err != nil {
			break
		}
		files = append(files, string(l))
	}

	parallel := 10
	todo := make(chan string, len(files))
	dts := make(chan int64, parallel)

	fmt.Printf("Statting %d files with %d threads\n", len(files), parallel)
	for i := 0 ; i < parallel; i++ {
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
	for i := 0 ; i < len(files); i++ {
		total += float64(<-dts) * 1e-6
	}

	fmt.Println("Average stat time (ms):", total/float64(len(files)))
}
