// Package qris membaca dan menulis payload QRIS (EMVCo MPM / ASPI).
package qris

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
)

var ErrMalformed = errors.New("QRIS tidak terbaca (format)")

type TLV struct {
	Tag   string
	Value string
}

// Parse memecah payload EMVCo jadi daftar TLV berurutan.
func Parse(payload string) ([]TLV, error) {
	var out []TLV
	i := 0
	for i < len(payload) {
		if i+4 > len(payload) {
			return nil, ErrMalformed
		}
		tag := payload[i : i+2]
		length, err := strconv.Atoi(payload[i+2 : i+4])
		if err != nil || length < 0 {
			return nil, ErrMalformed
		}
		start := i + 4
		end := start + length
		if end > len(payload) {
			return nil, ErrMalformed
		}
		out = append(out, TLV{Tag: tag, Value: payload[start:end]})
		i = end
	}
	if len(out) == 0 {
		return nil, ErrMalformed
	}
	return out, nil
}

// Serialize merangkai TLV jadi payload (Tag + Length 2 digit + Value).
func Serialize(items []TLV) string {
	var b []byte
	for _, t := range items {
		b = append(b, t.Tag...)
		b = append(b, []byte(fmt.Sprintf("%02d", len(t.Value)))...)
		b = append(b, t.Value...)
	}
	return string(b)
}

// Get mengambil value sebuah tag.
func Get(items []TLV, tag string) (string, bool) {
	for _, t := range items {
		if t.Tag == tag {
			return t.Value, true
		}
	}
	return "", false
}

func upsert(items []TLV, tag, value string) []TLV {
	for i := range items {
		if items[i].Tag == tag {
			items[i].Value = value
			return items
		}
	}
	return append(items, TLV{Tag: tag, Value: value})
}

func remove(items []TLV, tag string) []TLV {
	out := items[:0]
	for _, t := range items {
		if t.Tag != tag {
			out = append(out, t)
		}
	}
	return out
}

// sortByTag mengurutkan tag menaik — konvensi EMVCo, pastikan 54 sebelum 58.
func sortByTag(items []TLV) {
	sort.SliceStable(items, func(a, b int) bool {
		ta, _ := strconv.Atoi(items[a].Tag)
		tb, _ := strconv.Atoi(items[b].Tag)
		return ta < tb
	})
}
