// Package notif mengekstrak nominal dari teks notifikasi bank/e-wallet.
package notif

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
)

var (
	ErrNoMatch   = errors.New("nominal tidak ditemukan di notifikasi")
	ErrNoPattern = errors.New("tidak ada pola parser yang valid")
)

// Parser aman dipakai bersamaan dari banyak goroutine.
type Parser struct{ res []*regexp.Regexp }

// New mengompilasi pola parser. Tiap pola wajib punya satu capture group berisi
// angka nominal. Pola dicoba berurutan — format teks bank berubah sewaktu-waktu,
// jadi pola lama disimpan di belakang sebagai fallback. Pola yang gagal
// dikompilasi dilewati; error hanya kalau tak ada satu pun yang bisa dipakai.
func New(patterns []string) (*Parser, error) {
	var res []*regexp.Regexp
	for _, pat := range patterns {
		pat = strings.TrimSpace(pat)
		if pat == "" {
			continue
		}
		if re, err := regexp.Compile(pat); err == nil {
			res = append(res, re)
		}
	}
	if len(res) == 0 {
		return nil, ErrNoPattern
	}
	return &Parser{res: res}, nil
}

// NewFromTemplate menerima pola satu per baris dalam satu string.
func NewFromTemplate(template string) (*Parser, error) {
	return New(strings.Split(template, "\n"))
}

// ParseAmount mengekstrak nominal (Rupiah bulat) dari teks notifikasi.
func (p *Parser) ParseAmount(rawText string) (int64, error) {
	for _, re := range p.res {
		m := re.FindStringSubmatch(rawText)
		if len(m) < 2 {
			continue
		}
		digits := strings.NewReplacer(".", "", ",", "", " ", "").Replace(m[1])
		n, err := strconv.ParseInt(digits, 10, 64)
		if err != nil || n <= 0 {
			continue
		}
		return n, nil
	}
	return 0, ErrNoMatch
}
