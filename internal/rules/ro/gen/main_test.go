package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"
)

// TestUpToDate checks that rules_gen.go is the output of the generator for
// the schematron archive of FX_RO_SCH (conformance/fetch.sh).
func TestUpToDate(t *testing.T) {
	name := os.Getenv("FX_RO_SCH")
	if name == "" {
		t.Skip("FX_RO_SCH not set")
	}
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	if h := sha256.Sum256(data); hex.EncodeToString(h[:]) != sum {
		t.Fatalf("%s: SHA-256 %x, want %s", name, h, sum)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	s, err := read(zr)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.generate()
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("../rules_gen.go")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Error("rules_gen.go is not up to date: run go generate in internal/rules/ro")
	}
}
