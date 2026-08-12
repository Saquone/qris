// Command qris membungkus library ini jadi CLI, supaya bisa dipakai dari bahasa
// apa pun lewat exec — atau langsung dari shell saat debug QRIS di lapangan.
package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/saquone/qris"
	"github.com/saquone/qris/notif"
	"github.com/saquone/qris/qrimage"
)

const usage = `qris — konversi & validasi payload QRIS

  qris to-dynamic <nominal> [payload]   QRIS statis → dinamis bernominal
  qris validate [payload]               cek QRIS statis IDR (exit 0 = valid)
  qris merchant [payload]               cetak nama merchant (tag 59)
  qris decode <file.png|->              baca payload QR dari gambar
  qris parse <pola-regex>...            ekstrak nominal dari notifikasi (teks via stdin)

payload kosong = dibaca dari stdin.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	args := os.Args[2:]

	switch os.Args[1] {
	case "to-dynamic":
		if len(args) < 1 {
			fail("butuh nominal")
		}
		amount, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			fail("nominal bukan angka: %s", args[0])
		}
		out, err := qris.ToDynamic(input(args[1:]), amount)
		check(err)
		fmt.Println(out)

	case "validate":
		check(qris.ValidateStatic(input(args)))

	case "merchant":
		fmt.Println(qris.ExtractMerchantName(input(args)))

	case "decode":
		if len(args) < 1 {
			fail("butuh path file gambar (atau - untuk stdin)")
		}
		var data []byte
		var err error
		if args[0] == "-" {
			data, err = io.ReadAll(os.Stdin)
		} else {
			data, err = os.ReadFile(args[0])
		}
		check(err)
		payload, err := qrimage.Decode(data)
		check(err)
		fmt.Println(payload)

	case "parse":
		if len(args) < 1 {
			fail("butuh minimal satu pola regex")
		}
		p, err := notif.New(args)
		check(err)
		amount, err := p.ParseAmount(input(nil))
		check(err)
		fmt.Println(amount)

	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
}

// input mengambil argumen pertama, atau seluruh stdin bila argumen kosong.
func input(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	b, err := io.ReadAll(os.Stdin)
	check(err)
	return strings.TrimRight(string(b), "\r\n")
}

func check(err error) {
	if err != nil {
		fail("%v", err)
	}
}

func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "qris: "+format+"\n", a...)
	os.Exit(1)
}
