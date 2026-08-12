// Package uniq memilih kode unik yang ditempelkan ke nominal transaksi, supaya
// dua pembayaran bernominal sama bisa dibedakan dari teks notifikasi.
package uniq

import (
	"errors"
	"math/rand/v2"
)

var (
	ErrRange     = errors.New("rentang kode unik tidak valid")
	ErrExhausted = errors.New("semua kode unik pada rentang ini sedang dipakai")
)

// Pick memilih kode dari [min,max] yang belum menghasilkan nominal terpakai;
// taken = nominal final (base + kode) yang masih pending. Nominal manual =
// min == max.
//
// Kode dipilih acak, bukan berurutan: nominal berikutnya yang bisa ditebak
// memancing pembayaran nyasar ke transaksi orang lain.
func Pick(base int64, min, max int, taken []int64) (int, error) {
	if min < 0 || max < min {
		return 0, ErrRange
	}
	used := make(map[int64]bool, len(taken))
	for _, a := range taken {
		used[a] = true
	}
	var free []int
	for c := min; c <= max; c++ {
		if !used[base+int64(c)] {
			free = append(free, c)
		}
	}
	if len(free) == 0 {
		return 0, ErrExhausted
	}
	return free[rand.IntN(len(free))], nil
}
