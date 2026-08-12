package qris

import (
	"errors"
	"strconv"
)

var ErrInvalidAmount = errors.New("nominal tidak valid")

// ToDynamic mengubah QRIS statis jadi dinamis bernominal: tag 01→12, sisip 54,
// hitung ulang CRC. Identitas merchant tidak disentuh — tujuan dana tetap sama.
func ToDynamic(static string, amount int64) (string, error) {
	if amount <= 0 {
		return "", ErrInvalidAmount
	}
	items, err := Parse(static)
	if err != nil {
		return "", err
	}
	items = upsert(items, "01", "12")
	items = upsert(items, "54", strconv.FormatInt(amount, 10))
	items = remove(items, "63")
	sortByTag(items)

	body := Serialize(items) + "6304"
	return body + CRC16(body), nil
}

// ExtractMerchantName mengambil nama merchant (tag 59).
func ExtractMerchantName(payload string) string {
	items, err := Parse(payload)
	if err != nil {
		return ""
	}
	v, _ := Get(items, "59")
	return v
}
