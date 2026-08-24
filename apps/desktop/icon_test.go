//go:build darwin

package main

import (
	"bytes"
	"image/png"
	"testing"
)

func TestMenuBarIconIsValidTemplatePNG(t *testing.T) {
	decoded, err := png.Decode(bytes.NewReader(menuBarIcon()))
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Bounds().Dx() != 18 || decoded.Bounds().Dy() != 18 {
		t.Fatalf("icon bounds = %v", decoded.Bounds())
	}
}

func TestIntegerDecodesDaemonJSONNumbers(t *testing.T) {
	if integer(float64(7)) != 7 || integer("7") != 0 {
		t.Fatal("daemon number conversion was not fail-closed")
	}
}
