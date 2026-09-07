// Command generate-brand-icons renders the canonical Overgent app icon SVG's
// geometry into the PNG sizes required by Apple's iconutil.
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
)

type roundedRect struct{ x, y, width, height, radius float64 }

var mark = []roundedRect{
	{22, 5, 20, 4, 2}, {14, 10, 36, 4, 2}, {8, 15, 48, 4, 2},
	{5, 20, 22, 4, 2}, {37, 20, 22, 4, 2}, {3, 25, 18, 4, 2}, {43, 25, 18, 4, 2},
	{2, 30, 17, 4, 2}, {45, 30, 17, 4, 2}, {3, 35, 18, 4, 2}, {43, 35, 18, 4, 2},
	{5, 40, 22, 4, 2}, {37, 40, 22, 4, 2}, {8, 45, 48, 4, 2},
	{14, 50, 36, 4, 2}, {22, 55, 20, 4, 2},
}

func main() {
	if len(os.Args) < 2 || len(os.Args) > 3 {
		fmt.Fprintln(os.Stderr, "usage: generate-brand-icons ICONSET_DIRECTORY [ICNS_FILE]")
		os.Exit(2)
	}
	if err := os.MkdirAll(os.Args[1], 0o755); err != nil {
		panic(err)
	}
	outputs := []struct {
		name string
		size int
	}{
		{"icon_16x16.png", 16}, {"icon_16x16@2x.png", 32},
		{"icon_32x32.png", 32}, {"icon_32x32@2x.png", 64},
		{"icon_128x128.png", 128}, {"icon_128x128@2x.png", 256},
		{"icon_256x256.png", 256}, {"icon_256x256@2x.png", 512},
		{"icon_512x512.png", 512}, {"icon_512x512@2x.png", 1024},
	}
	for _, output := range outputs {
		if err := writePNG(filepath.Join(os.Args[1], output.name), render(output.size)); err != nil {
			panic(err)
		}
	}
	if len(os.Args) == 3 {
		if err := writeICNS(os.Args[1], os.Args[2]); err != nil {
			panic(err)
		}
	}
}

func render(size int) image.Image {
	const samples = 4
	canvas := image.NewNRGBA(image.Rect(0, 0, size, size))
	for py := 0; py < size; py++ {
		for px := 0; px < size; px++ {
			var insideMark int
			for sy := 0; sy < samples; sy++ {
				for sx := 0; sx < samples; sx++ {
					x := (float64(px) + (float64(sx)+0.5)/samples) * 1024 / float64(size)
					y := (float64(py) + (float64(sy)+0.5)/samples) * 1024 / float64(size)
					for _, bar := range mark {
						scaled := roundedRect{62 + bar.x*14, 60 + bar.y*14, bar.width * 14, bar.height * 14, bar.radius * 14}
						if contains(scaled, x, y) {
							insideMark++
							break
						}
					}
				}
			}
			coverage := uint8(insideMark * 255 / (samples * samples))
			if coverage == 0 {
				continue
			}
			canvas.SetNRGBA(px, py, color.NRGBA{14, 14, 14, coverage})
		}
	}
	return canvas
}

func contains(rect roundedRect, x, y float64) bool {
	if x < rect.x || y < rect.y || x > rect.x+rect.width || y > rect.y+rect.height {
		return false
	}
	cx := min(max(x, rect.x+rect.radius), rect.x+rect.width-rect.radius)
	cy := min(max(y, rect.y+rect.radius), rect.y+rect.height-rect.radius)
	dx, dy := x-cx, y-cy
	return dx*dx+dy*dy <= rect.radius*rect.radius
}

func writePNG(path string, value image.Image) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return png.Encode(file, value)
}

// Modern icns entries are PNG payloads in a small big-endian container. Keep
// every physical size so Finder, the Dock, Spotlight, and old macOS APIs never
// have to upscale a smaller representation.
func writeICNS(iconset, destination string) error {
	entries := []struct{ kind, file string }{
		{"icp4", "icon_16x16.png"},
		{"icp5", "icon_32x32.png"},
		{"icp6", "icon_32x32@2x.png"},
		{"ic07", "icon_128x128.png"},
		{"ic08", "icon_256x256.png"},
		{"ic09", "icon_512x512.png"},
		{"ic10", "icon_512x512@2x.png"},
	}
	var body bytes.Buffer
	for _, entry := range entries {
		payload, err := os.ReadFile(filepath.Join(iconset, entry.file))
		if err != nil {
			return err
		}
		body.WriteString(entry.kind)
		if err := binary.Write(&body, binary.BigEndian, uint32(len(payload)+8)); err != nil {
			return err
		}
		body.Write(payload)
	}
	var result bytes.Buffer
	result.WriteString("icns")
	if err := binary.Write(&result, binary.BigEndian, uint32(body.Len()+8)); err != nil {
		return err
	}
	result.Write(body.Bytes())
	return os.WriteFile(destination, result.Bytes(), 0o644)
}
