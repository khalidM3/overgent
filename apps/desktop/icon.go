package main

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
)

// overgentMarkPNG draws the Overgent mark - the open O formed by horizontal
// radar lines, the exact mirrored geometry of assets/brand/overgent-mark.svg -
// at status-icon size, in one ink colour on transparency.
//
// The mark has no container and takes a single ink, which is the design
// system's Rule 1: black on a light surface, white on a dark one. macOS asks
// for the black one as a template image and tints it itself; Linux and Windows
// are handed both and choose by their own theme.
func overgentMarkPNG(ink color.Color) []byte {
	canvas := image.NewRGBA(image.Rect(0, 0, 18, 18))
	bar := func(x, y, width int) {
		draw.Draw(canvas, image.Rect(x, y, x+width, y+1), &image.Uniform{C: ink}, image.Point{}, draw.Src)
	}
	bar(7, 1, 4)
	bar(4, 3, 10)
	bar(2, 5, 14)
	bar(1, 7, 6)
	bar(11, 7, 6)
	bar(1, 9, 5)
	bar(12, 9, 5)
	bar(1, 11, 6)
	bar(11, 11, 6)
	bar(2, 13, 14)
	bar(4, 15, 10)
	bar(7, 17, 4)
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, canvas); err != nil {
		return nil
	}
	return encoded.Bytes()
}

var (
	markInk   = color.RGBA{A: 255}
	markPaper = color.RGBA{R: 255, G: 255, B: 255, A: 255}
)
