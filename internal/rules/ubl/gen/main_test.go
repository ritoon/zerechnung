package main

import (
	"bytes"
	"os"
	"testing"
)

// TestCTCFRUpToDate checks that ctcfr_gen.go is the output of the generator
// for the EXTENDED-CTC-FR schematron for UBL of FX_FNFE, a checkout of
// github.com/fnfempe/France_RFE at tag v1.4.0.04.
func TestCTCFRUpToDate(t *testing.T) {
	dir := os.Getenv("FX_FNFE")
	if dir == "" {
		t.Skip("FX_FNFE not set")
	}
	got, err := generateCTCFR(dir)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("../ctcfr_gen.go")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Error("ctcfr_gen.go is not up to date: run go generate in internal/rules/ubl")
	}
}
