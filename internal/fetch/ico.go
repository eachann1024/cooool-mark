package fetch

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
)

// icoToPNG extracts the largest icon entry from an ICO container and
// returns PNG bytes when possible.
func icoToPNG(data []byte) ([]byte, bool) {
	if len(data) < 6 {
		return nil, false
	}
	reserved := binary.LittleEndian.Uint16(data[0:2])
	typ := binary.LittleEndian.Uint16(data[2:4])
	count := int(binary.LittleEndian.Uint16(data[4:6]))
	if reserved != 0 || (typ != 1 && typ != 2) || count <= 0 || len(data) < 6+count*16 {
		return nil, false
	}

	bestIdx := -1
	bestScore := -1
	for i := 0; i < count; i++ {
		off := 6 + i*16
		w := int(data[off])
		if w == 0 {
			w = 256
		}
		h := int(data[off+1])
		if h == 0 {
			h = 256
		}
		bpp := int(binary.LittleEndian.Uint16(data[off+6 : off+8]))
		size := int(binary.LittleEndian.Uint32(data[off+8 : off+12]))
		imgOff := int(binary.LittleEndian.Uint32(data[off+12 : off+16]))
		if size <= 0 || imgOff < 0 || imgOff+size > len(data) {
			continue
		}
		score := w*h*100 + bpp
		if score > bestScore {
			bestScore = score
			bestIdx = i
		}
	}
	if bestIdx < 0 {
		return nil, false
	}

	off := 6 + bestIdx*16
	size := int(binary.LittleEndian.Uint32(data[off+8 : off+12]))
	imgOff := int(binary.LittleEndian.Uint32(data[off+12 : off+16]))
	entry := data[imgOff : imgOff+size]

	if bytes.HasPrefix(entry, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}) {
		return entry, true
	}
	return dibToPNG(entry, int(data[off]), int(data[off+1]))
}

func dibToPNG(dib []byte, entryW, entryH int) ([]byte, bool) {
	if len(dib) < 40 {
		return nil, false
	}
	hdrSize := int(binary.LittleEndian.Uint32(dib[0:4]))
	if hdrSize < 40 || len(dib) < hdrSize {
		return nil, false
	}
	w := int(int32(binary.LittleEndian.Uint32(dib[4:8])))
	hTotal := int(int32(binary.LittleEndian.Uint32(dib[8:12])))
	bpp := int(binary.LittleEndian.Uint16(dib[14:16]))
	comp := binary.LittleEndian.Uint32(dib[16:20])
	if comp != 0 {
		return nil, false
	}
	if w <= 0 {
		w = entryW
		if w == 0 {
			w = 256
		}
	}
	h := hTotal / 2
	if h <= 0 {
		h = entryH
		if h == 0 {
			h = 256
		}
	}
	if w <= 0 || h <= 0 || w > 1024 || h > 1024 {
		return nil, false
	}

	pixels := dib[hdrSize:]
	img := image.NewNRGBA(image.Rect(0, 0, w, h))

	switch bpp {
	case 32:
		rowBytes := w * 4
		if len(pixels) < rowBytes*h {
			return nil, false
		}
		for y := 0; y < h; y++ {
			srcY := h - 1 - y
			row := pixels[srcY*rowBytes : (srcY+1)*rowBytes]
			for x := 0; x < w; x++ {
				b, g, r, a := row[x*4], row[x*4+1], row[x*4+2], row[x*4+3]
				img.SetNRGBA(x, y, color.NRGBA{R: r, G: g, B: b, A: a})
			}
		}
	case 24:
		rowStride := ((w*3 + 3) / 4) * 4
		andStride := (((w + 31) / 32) * 4)
		xorSize := rowStride * h
		if len(pixels) < xorSize {
			return nil, false
		}
		hasAnd := len(pixels) >= xorSize+andStride*h
		andMask := pixels[xorSize:]
		for y := 0; y < h; y++ {
			srcY := h - 1 - y
			row := pixels[srcY*rowStride : (srcY+1)*rowStride]
			for x := 0; x < w; x++ {
				a := uint8(255)
				if hasAnd {
					bit := (andMask[srcY*andStride+x/8] >> uint(7-(x%8))) & 1
					if bit == 1 {
						a = 0
					}
				}
				b, g, r := row[x*3], row[x*3+1], row[x*3+2]
				img.SetNRGBA(x, y, color.NRGBA{R: r, G: g, B: b, A: a})
			}
		}
	default:
		return nil, false
	}

	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return nil, false
	}
	return out.Bytes(), true
}
