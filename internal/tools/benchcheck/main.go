// Command benchcheck compares a benchmark run with a baseline run and fails when a case is slower by more than the allowed fraction: go run ./internal/tools/benchcheck [-max 0.3] baseline.txt run.txt. It reads the output of go test -bench and compares medians of ns/op, so a single noisy run does not decide. CI gives it the base and the head of a change benchmarked on the same runner (.github/workflows/bench.yml); locally, the baseline is testdata/bench/baseline.txt.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

func main() {
	maxSlower := flag.Float64("max", 0.3, "largest allowed slowdown, as a fraction of the baseline")
	flag.Parse()
	if flag.NArg() != 2 {
		fmt.Fprintln(os.Stderr, "usage: benchcheck [-max 0.3] baseline.txt run.txt")
		os.Exit(2)
	}
	base, err := read(flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	run, err := read(flag.Arg(1))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	names := make([]string, 0, len(base))
	for n := range base {
		names = append(names, n)
	}
	sort.Strings(names)
	failed := false
	for _, n := range names {
		b, r := median(base[n]), median(run[n])
		if len(run[n]) == 0 {
			fmt.Printf("%-40s baseline %12.0f ns/op   not run\n", n, b)
			continue
		}
		change := r/b - 1
		mark := ""
		if change > *maxSlower {
			mark = "   SLOWER"
			failed = true
		}
		fmt.Printf("%-40s baseline %12.0f ns/op   now %12.0f ns/op   %+6.1f%%%s\n", n, b, r, 100*change, mark)
	}
	if failed {
		fmt.Printf("\nslower than the baseline by more than %.0f%%; see AGENTS.md (a slowdown needs a reason in the commit message, and a meant one re-records testdata/bench/baseline.txt)\n", 100**maxSlower)
		os.Exit(1)
	}
}

// read collects ns/op by benchmark name, with the -N CPU suffix dropped.
func read(path string) (map[string][]float64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[string][]float64{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 4 || !strings.HasPrefix(fields[0], "Benchmark") || fields[3] != "ns/op" {
			continue
		}
		name := fields[0]
		if i := strings.LastIndex(name, "-"); i > 0 {
			name = name[:i]
		}
		v, err := strconv.ParseFloat(fields[2], 64)
		if err != nil {
			continue
		}
		out[name] = append(out[name], v)
	}
	return out, sc.Err()
}

func median(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	return s[len(s)/2]
}
