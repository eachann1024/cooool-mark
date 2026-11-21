package theme

import (
	"bytes"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"sync"

	"github.com/egoist/mygo/ui"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"
)

// IconTone says how an icon's visible pixels read against a plain
// background: a mostly white glyph on transparency vanishes on the light
// card, a mostly black one on the dark card.
type IconTone uint8

const (
	ToneMixed IconTone = iota
	ToneLight
	ToneDark
)

var (
	toneMu    sync.Mutex
	toneCache = map[string]IconTone{}
)

// LoadIconTone measures an icon file once.
func LoadIconTone(path string) IconTone {
	if path == "" {
		return ToneMixed
	}
	toneMu.Lock()
	defer toneMu.Unlock()
	if t, ok := toneCache[path]; ok {
		return t
	}
	t := ToneMixed
	if data, err := os.ReadFile(path); err == nil {
		t = measureTone(data)
	}
	toneCache[path] = t
	return t
}

func measureTone(data []byte) IconTone {
	var img image.Image
	if m, _, err := image.Decode(bytes.NewReader(data)); err == nil {
		img = m
	} else if s, err := ui.ParseSVG(data); err == nil {
		img = s.Image(48, 48, color.Black)
	} else {
		return ToneMixed
	}
	return classifyTone(img)
}

// classifyTone samples up to 64×64 points. An icon counts as light (dark)
// only when a fair share of it is see-through, so the card shows through,
// and nearly all of what is visible is near white (near black).
func classifyTone(img image.Image) IconTone {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return ToneMixed
	}
	step := max(1, max(w, h)/64)
	var total, clear, vis, light, dark float64
	for y := b.Min.Y; y < b.Max.Y; y += step {
		for x := b.Min.X; x < b.Max.X; x += step {
			total++
			r, g, bl, a := img.At(x, y).RGBA()
			if a < 0x4000 {
				clear++
				continue
			}
			af := float64(a)
			// Un-premultiply to the pixel's own color.
			lum := (0.2126*float64(r) + 0.7152*float64(g) + 0.0722*float64(bl)) / af
			wgt := af / 0xffff
			vis += wgt
			switch {
			case lum > 0.82:
				light += wgt
			case lum < 0.22:
				dark += wgt
			}
		}
	}
	if total == 0 || vis == 0 || clear/total < 0.12 {
		return ToneMixed
	}
	switch {
	case light/vis >= 0.85:
		return ToneLight
	case dark/vis >= 0.85:
		return ToneDark
	}
	return ToneMixed
}
