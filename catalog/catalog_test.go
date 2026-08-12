package catalog_test

import (
	"regexp"
	"testing"

	"github.com/saquone/qris/catalog"
	"github.com/saquone/qris/notif"
)

// Katalog ini dikirim ke aplikasi Android dan dipakai dua mesin regex berbeda: RE2 (Go) dan
// regex Java (Kotlin). Test ini menjaga tiap pola tetap valid di RE2, punya tepat satu capture
// group, dan benar-benar mengekstrak nominal — pola rusak akan diam-diam mematikan satu gateway.
func TestGatewaysWellFormed(t *testing.T) {
	keys := map[string]bool{}
	pkgs := map[string]string{}
	for _, g := range catalog.All() {
		if keys[g.Key] {
			t.Errorf("key ganda: %s", g.Key)
		}
		keys[g.Key] = true

		if len(g.Packages) == 0 || len(g.Patterns) == 0 {
			t.Errorf("%s: packages/patterns kosong", g.Key)
		}
		for _, p := range g.Packages {
			if prev, dup := pkgs[p]; dup {
				t.Errorf("package %s dipakai %s dan %s", p, prev, g.Key)
			}
			pkgs[p] = g.Key
		}
		for _, pat := range g.Patterns {
			re, err := regexp.Compile(pat)
			if err != nil {
				t.Errorf("%s: pola tidak valid %q: %v", g.Key, pat, err)
				continue
			}
			if n := re.NumSubexp(); n != 1 {
				t.Errorf("%s: pola %q punya %d capture group, harus tepat 1", g.Key, pat, n)
			}
		}
	}
}

func TestParserExtractsAmount(t *testing.T) {
	p, err := catalog.Parser()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		text string
		want int64
	}{
		{"Rp50.137 diterima DANA Bisnis.", 50137},
		{"Kamu menerima Rp 75.000 dari pelanggan", 75000},
		{"Pembayaran QRIS sebesar Rp1.250.000 berhasil", 1250000},
	} {
		got, err := p.ParseAmount(tc.text)
		if err != nil || got != tc.want {
			t.Errorf("ParseAmount(%q) = %d, %v — mau %d", tc.text, got, err, tc.want)
		}
	}
	if _, err := p.ParseAmount("Diskon 50% hari ini, buruan!"); err != notif.ErrNoMatch {
		t.Errorf("teks promo ikut ter-parse, err = %v", err)
	}
}

func TestByPackage(t *testing.T) {
	if g, ok := catalog.ByPackage("com.gojek.gopaymerchant"); !ok || g.Key != "gopay" {
		t.Errorf("ByPackage(gopaymerchant) = %q, %v", g.Key, ok)
	}
	if _, ok := catalog.ByPackage("com.contoh.tidakada"); ok {
		t.Error("paket asing dikenali sebagai gateway")
	}
}
