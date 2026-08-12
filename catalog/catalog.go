// Package catalog adalah satu-satunya sumber kebenaran tentang aplikasi mana yang didukung dan
// pola apa yang dipakai untuk membaca nominal dari notifikasinya.
//
// Aplikasi Android tidak menyimpan daftarnya sendiri — dia mengambil katalog ini lewat
// `GET /gateways` di qris-server, lalu menyimpannya untuk dipakai saat offline.
package catalog

import (
	_ "embed"
	"encoding/json"

	"github.com/saquone/qris/notif"
)

//go:embed gateways.json
var raw []byte

type Gateway struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	// Packages = nama paket Android. Satu gateway bisa punya beberapa (versi konsumen vs merchant).
	Packages []string `json:"packages"`
	// Patterns dicoba berurutan; tiap pola wajib punya satu capture group berisi angka.
	Patterns []string `json:"patterns"`
}

var gateways = func() []Gateway {
	var g []Gateway
	if err := json.Unmarshal(raw, &g); err != nil {
		panic("catalog: gateways.json rusak: " + err.Error())
	}
	return g
}()

func All() []Gateway { return gateways }

// Raw mengembalikan JSON katalog apa adanya — dipakai server saat menyajikan `GET /gateways`.
func Raw() []byte { return raw }

func ByPackage(pkg string) (Gateway, bool) {
	for _, g := range gateways {
		for _, p := range g.Packages {
			if p == pkg {
				return g, true
			}
		}
	}
	return Gateway{}, false
}

// Parser menggabungkan pola SEMUA gateway jadi satu, dicoba berurutan. Dipakai saat pemanggil
// tidak peduli notifikasi datang dari aplikasi mana — pola yang cocok duluan yang menang.
func Parser() (*notif.Parser, error) {
	var patterns []string
	for _, g := range gateways {
		patterns = append(patterns, g.Patterns...)
	}
	return notif.New(patterns)
}
