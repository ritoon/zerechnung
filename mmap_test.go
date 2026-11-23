//go:build unix

package zerechnung

import (
	"os"
	"runtime/debug"
	"syscall"
	"testing"

	"github.com/ritoon/zerechnung/cii"
	"github.com/ritoon/zerechnung/validate"
)

// mapReadOnly maps a file in memory, read only: writing to the returned
// slice faults.
func mapReadOnly(t *testing.T, name string) []byte {
	t.Helper()
	f, err := os.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	data, err := syscall.Mmap(int(f.Fd()), 0, int(fi.Size()), syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		t.Skip("mmap:", err)
	}
	t.Cleanup(func() { _ = syscall.Munmap(data) })
	return data
}

// TestReadOnlyInput checks that the package never writes to the slices it
// is given: the inputs are files mapped read only, which a write would make
// fault. Stream.Raw points into the input, so a PDF mapped with mmap is
// read without being copied to the heap.
func TestReadOnlyInput(t *testing.T) {
	// A fault becomes a panic of the goroutine that writes, with the
	// address, instead of a crash of the test binary.
	defer debug.SetPanicOnFault(debug.SetPanicOnFault(true))
	pdf := mapReadOnly(t, "testdata/EN16931_Einfach.pdf")
	host := mapReadOnly(t, "testdata/host.pdf")
	xml := mapReadOnly(t, "testdata/EN16931_Einfach.cii.xml")

	if _, err := Extract(pdf); err != nil {
		t.Fatal("Extract:", err)
	}
	if _, err := ExtractAll(pdf); err != nil {
		t.Fatal("ExtractAll:", err)
	}
	if _, _, err := Read(pdf); err != nil {
		t.Fatal("Read:", err)
	}
	for _, level := range []validate.PDFALevel{validate.PDFAFull, validate.PDFABasic} {
		if _, att, err := Check(pdf, &validate.Options{PDFA: level}); err != nil || att == nil {
			t.Fatal("Check:", att, err)
		}
	}
	if _, _, err := CheckContainer(pdf); err != nil {
		t.Fatal("CheckContainer:", err)
	}
	if _, _, err := VerifySignatures(pdf, nil); err != nil {
		t.Fatal("VerifySignatures:", err)
	}
	if _, err := Embed(host, xml, nil); err != nil {
		t.Fatal("Embed:", err)
	}
	if _, err := cii.Parse(xml); err != nil {
		t.Fatal("cii.Parse:", err)
	}
	validate.XML(xml, nil)
}
