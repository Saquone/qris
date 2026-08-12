package qris_test

import (
	"strings"
	"testing"

	"github.com/saquone/qris"
)

// QRIS statis contoh, merchant dummy. CRC di ekornya mengunci implementasi CRC16.
const static = "00020101021126650014COM.GO-JEK.WWW01189360091400000000000215000000000000000303UMI51440014ID.CO.QRIS.WWW0215ID10200000000000303UMI5204581253033605802ID5909TOKO DEMO6007JAKARTA61051234062070703A0163048748"

func TestValidateStatic(t *testing.T) {
	if err := qris.ValidateStatic(static); err != nil {
		t.Fatalf("QRIS statis valid ditolak: %v", err)
	}
	if err := qris.ValidateStatic(strings.Replace(static, "5909TOKO", "5909ROKO", 1)); err != qris.ErrInvalidCRC {
		t.Fatalf("payload rusak lolos, err = %v", err)
	}
}

func TestToDynamic(t *testing.T) {
	out, err := qris.ToDynamic(static, 50137)
	if err != nil {
		t.Fatal(err)
	}
	items, err := qris.Parse(out)
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := qris.Get(items, "01"); v != "12" {
		t.Errorf("point of initiation = %q, mau 12 (dinamis)", v)
	}
	if v, _ := qris.Get(items, "54"); v != "50137" {
		t.Errorf("nominal = %q, mau 50137", v)
	}
	if v, _ := qris.Get(items, "59"); v != "TOKO DEMO" {
		t.Errorf("nama merchant berubah jadi %q — identitas merchant tak boleh disentuh", v)
	}
	// CRC hasil harus konsisten dengan isinya.
	body := out[:len(out)-4]
	if got := qris.CRC16(body); got != out[len(out)-4:] {
		t.Errorf("CRC hasil = %s, hitung ulang = %s", out[len(out)-4:], got)
	}
	if _, err := qris.ToDynamic(static, 0); err != qris.ErrInvalidAmount {
		t.Errorf("nominal 0 diterima, err = %v", err)
	}
}
