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
