package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestUpToDate checks that rules_gen.go is the output of the generator for
// the schematron of the checkout of France_RFE in FX_FNFE
// (conformance/fetch.sh).
func TestUpToDate(t *testing.T) {
	root := os.Getenv("FX_FNFE")
	if root == "" {
		t.Skip("FX_FNFE not set")
	}
	data, err := os.ReadFile(filepath.Join(root, dir, mainFile))
	if err != nil {
		t.Fatal(err)
	}
	got, err := generate(data)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("../rules_gen.go")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Error("rules_gen.go is not up to date: run go generate in internal/rules/cdv")
	}
}

// TestGenerateErrors checks that the generator refuses a file whose SHA-256
// is not the one of the schematron, and a file that is not a schematron.
func TestGenerateErrors(t *testing.T) {
	if _, err := generate([]byte("<schema/>")); err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Errorf("generate: %v, want a SHA-256 error", err)
	}
	if _, err := read([]byte("<x")); err == nil {
		t.Error("read accepted a truncated document")
	}
}
