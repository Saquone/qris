// Package qrimage membaca payload QRIS dari gambar PNG/JPEG.
package qrimage

import (
	"bytes"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"

	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"
)

var ErrNoQR = errors.New("QR tidak terbaca. Foto lebih jelas/lurus.")

// MaxPixels menolak gambar berdimensi raksasa sebelum buffer piksel dialokasi:
// file 20 KB yang mendeklarasikan 15000x15000 minta ~900 MB dan bisa OOM.
var MaxPixels int64 = 25_000_000

// Decode membaca payload QR dari bytes gambar.
func Decode(data []byte) (string, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || int64(cfg.Width)*int64(cfg.Height) > MaxPixels {
		return "", ErrNoQR
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", ErrNoQR
	}
	bmp, err := gozxing.NewBinaryBitmapFromImage(img)
	if err != nil {
		return "", ErrNoQR
	}
	res, err := qrcode.NewQRCodeReader().Decode(bmp, nil)
	if err != nil {
		return "", ErrNoQR
	}
	return res.GetText(), nil
}
