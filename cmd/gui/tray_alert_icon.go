package main

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
)

// alertIconPNG returns the tray icon with a red dot over its bottom-right
// quarter: the "glow.moe is not receiving anything" state.
func alertIconPNG(base []byte) ([]byte, error) {
	src, err := png.Decode(bytes.NewReader(base))
	if err != nil {
		return nil, err
	}
	b := src.Bounds()
	img := image.NewNRGBA(b)
	draw.Draw(img, b, src, b.Min, draw.Src)
	w := float64(b.Dx())
	cx, cy, r := w*0.74, w*0.74, w*0.24
	red := color.NRGBA{R: 0xff, G: 0x3b, B: 0x5c, A: 0xff}
	white := color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			dx, dy := float64(x-b.Min.X)+0.5-cx, float64(y-b.Min.Y)+0.5-cy
			d := math.Sqrt(dx*dx + dy*dy)
			switch {
			case d <= r:
				img.SetNRGBA(x, y, red)
			case d <= r+math.Max(1, w/32):
				img.SetNRGBA(x, y, white)
			}
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
