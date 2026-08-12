package qris

import (
	"errors"
	"strings"
)

var (
	ErrInvalidCRC = errors.New("QRIS tidak valid (checksum)")
	ErrMissingTag = errors.New("QRIS tidak lengkap")
	ErrNotStatic  = errors.New("Ini QRIS dinamis, butuh QRIS statis")
	ErrNotIDR     = errors.New("QRIS bukan mata uang Rupiah")
)

// ValidateStatic memastikan payload = QRIS statis IDR dengan CRC benar.
func ValidateStatic(payload string) error {
	if len(payload) < 8 {
		return ErrMalformed
	}
	body := payload[:len(payload)-4] // CRC dihitung sampai (termasuk) "6304"
	if body[len(body)-4:] != "6304" {
		return ErrMalformed
	}
	if CRC16(body) != strings.ToUpper(payload[len(payload)-4:]) {
		return ErrInvalidCRC
	}
	items, err := Parse(payload)
	if err != nil {
		return err
	}
	for _, tag := range []string{"00", "01", "53", "58", "59", "63"} {
		if _, ok := Get(items, tag); !ok {
			return ErrMissingTag
		}
	}
	if v, _ := Get(items, "01"); v != "11" {
		return ErrNotStatic
	}
	if v, _ := Get(items, "53"); v != "360" {
		return ErrNotIDR
	}
	return nil
}
