//go:build unix

package zerechnung_test

import (
	"fmt"
	"log"
	"os"
	"syscall"

	"github.com/ritoon/zerechnung"
)

// Example_mmap reads a PDF mapped in memory instead of loaded with
// os.ReadFile: the package never writes to its input, and the streams it
// reads point into it, so a large PDF is not copied to the heap. The
// standard library maps files through package syscall, on Unix systems
// only; on Windows, use os.ReadFile.
func Example_mmap() {
	f, err := os.Open("testdata/EN16931_Einfach.pdf")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		log.Fatal(err)
	}
	pdf, err := syscall.Mmap(int(f.Fd()), 0, int(fi.Size()), syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		log.Fatal(err)
	}
	defer syscall.Munmap(pdf)

	att, err := zerechnung.Extract(pdf)
	if err != nil {
		log.Fatal(err)
	}
	// The attachment holds copies: it stays valid once the file is unmapped.
	fmt.Println(att.Filename, len(att.XML) > 0)
	// Output: factur-x.xml true
}
