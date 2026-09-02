// zipmake.go — builds the zips the E2E suite needs. Kept as a standalone
// single-file tool (no module) so `go build ./...` of the backend stays clean.
//
// Usage: go run scripts/zipmake.go -mode <testdata|traversal> -out <file.zip>
//
//   testdata   1.in/1.out/2.in/2.out pairs for the normal upload flow
//   traversal  a zip whose entry name tries to escape the target dir
//              (../../evil.txt) plus one legitimate case pair; used to prove
//              the server never writes outside its data root
package main

import (
	"archive/zip"
	"flag"
	"fmt"
	"os"
)

type entry struct{ name, body string }

func main() {
	mode := flag.String("mode", "testdata", "testdata | traversal")
	out := flag.String("out", "out.zip", "destination zip path")
	flag.Parse()

	var entries []entry
	switch *mode {
	case "testdata":
		entries = []entry{
			{"1.in", "1 2\n"}, {"1.out", "3\n"},
			{"2.in", "10 20\n"}, {"2.out", "30\n"},
		}
	case "traversal":
		entries = []entry{
			{"../../evil.txt", "pwned-by-traversal"},
			{"1.in", "5 5\n"}, {"1.out", "10\n"},
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown mode %q\n", *mode)
		os.Exit(2)
	}

	f, err := os.Create(*out)
	if err != nil {
		fmt.Fprintln(os.Stderr, "create:", err)
		os.Exit(1)
	}
	defer f.Close()
	w := zip.NewWriter(f)
	for _, e := range entries {
		fh := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
		ew, err := w.CreateHeader(fh)
		if err != nil {
			fmt.Fprintln(os.Stderr, "zip entry:", err)
			os.Exit(1)
		}
		if _, err := ew.Write([]byte(e.body)); err != nil {
			fmt.Fprintln(os.Stderr, "zip write:", err)
			os.Exit(1)
		}
	}
	if err := w.Close(); err != nil {
		fmt.Fprintln(os.Stderr, "zip close:", err)
		os.Exit(1)
	}
}
