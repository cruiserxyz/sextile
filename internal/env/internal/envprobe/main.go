// Command envprobe prints what internal/env and internal/store resolve in the
// build it was compiled as, so a test can build it with and without -tags dev
// and compare (the specification C1-C5). It is test support: nothing
// ships it, and it starts no process.
//
//	envprobe get KEY...    one KEY=value line per key ("KEY=" when unset)
//	envprobe resolve PATH  env.ResolveDir(PATH)
//	envprobe charts        store.ChartsDir()
//	envprobe noload KEY... like get, but never calls env.Load: what merely importing env does
package main

import (
	"fmt"
	"os"

	"github.com/cruiserxyz/sextile/internal/env"
	"github.com/cruiserxyz/sextile/internal/store"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: envprobe get KEY... | resolve PATH | charts")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "noload":
		for _, k := range os.Args[2:] {
			fmt.Printf("%s=%s\n", k, os.Getenv(k))
		}
		return
	case "get":
		env.Load() // importing env no longer loads .env by itself
		for _, k := range os.Args[2:] {
			fmt.Printf("%s=%s\n", k, os.Getenv(k))
		}
	case "resolve":
		env.Load()
		if len(os.Args) != 3 {
			fmt.Fprintln(os.Stderr, "usage: envprobe resolve PATH")
			os.Exit(2)
		}
		fmt.Println(env.ResolveDir(os.Args[2]))
	case "charts":
		env.Load()
		fmt.Println(store.ChartsDir())
	default:
		fmt.Fprintf(os.Stderr, "envprobe: unknown mode %q\n", os.Args[1])
		os.Exit(2)
	}
}
