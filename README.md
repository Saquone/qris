# saquone/qris

Library Go untuk **QRIS**: ubah QRIS statis jadi QRIS dinamis bernominal, validasi
payload, baca QR dari gambar, ekstrak nominal dari notifikasi bank, dan kirim
webhook bertanda tangan.

Diekstrak dari mesin pembayaran [saquone](https://github.com/Saquone). Tanpa
database, tanpa akun, tanpa API key. Lisensi MIT.

Bisa dipakai lewat **3 cara**: library Go, CLI, dan HTTP API.

---

## 1. Library Go

```bash
go get github.com/saquone/qris
```

Butuh Go 1.22+.

### Konversi & validasi

```go
import "github.com/saquone/qris"

// Tolak yang bukan QRIS statis IDR (CRC salah, tag kurang, sudah dinamis, non-IDR).
if err := qris.ValidateStatic(statis); err != nil {
    return err
}

// tag 01: 11 → 12, sisip tag 54 nominal, CRC (tag 63) dihitung ulang.
dinamis, err := qris.ToDynamic(statis, 50137)

nama := qris.ExtractMerchantName(statis) // "TOKO DEMO"

items, _ := qris.Parse(statis)
mcc, _ := qris.Get(items, "52")
```

Identitas merchant dan tujuan dana **tidak disentuh** — yang ditambahkan hanya
nominal, jadi uang tetap masuk ke rekening yang sama dengan QRIS statis aslinya.

Error: `ErrMalformed`, `ErrInvalidCRC`, `ErrMissingTag`, `ErrNotStatic`,
`ErrNotIDR`, `ErrInvalidAmount`.

### QR dari gambar

```go
import "github.com/saquone/qris/qrimage"

payload, err := qrimage.Decode(bytesPNG) // PNG atau JPEG
```

`qrimage.MaxPixels` (default ~25 MP) menolak gambar berdimensi raksasa sebelum
buffer piksel dialokasi. Naikkan kalau host-mu lega.

### Nominal dari notifikasi

Pola di-inject (dari DB, config, apa pun) dan dicoba **berurutan**, jadi pola lama
tetap jalan sebagai fallback saat format teks bank berubah.

```go
import "github.com/saquone/qris/notif"

p, _ := notif.New([]string{
    `menerima Rp ?([0-9.,]+)`,    // format baru
    `Dana masuk Rp\.?([0-9.,]+)`, // format lama, fallback
})
amount, err := p.ParseAmount("DANA: Anda menerima Rp 50.137 dari BUDI") // 50137
```

`notif.NewFromTemplate(blok)` menerima pola satu-per-baris dalam satu string —
enak disimpan di satu kolom DB.

Notifikasi sering dikirim ulang oleh OS. Simpan `DedupKey` di kolom
ber-unique-index; insert yang tertolak = duplikat:

```go
key := notif.DedupKey(userID, rawText, amount, capturedAt, time.Minute)
```

### Kode unik

Dua pembayaran bernominal sama tidak bisa dibedakan dari teks notifikasi.
Tempelkan kode kecil ke nominal (Rp50.000 → Rp50.137) supaya tiap transaksi
pending unik.

```go
import "github.com/saquone/qris/uniq"

// taken = nominal final transaksi yang masih pending.
kode, err := uniq.Pick(50000, 1, 999, []int64{50137, 50250})
nominalFinal := 50000 + int64(kode)
```

Kode dipilih acak dari sisa yang bebas. `ErrExhausted` kalau rentang habis.
Nominal manual = `min == max`.

### Webhook

```go
import "github.com/saquone/qris/webhook"
```

**Kirim** — client-nya dikeraskan terhadap SSRF: hostname di-resolve sendiri lalu
IP hasil validasi yang di-dial (tahan DNS rebinding), IP privat/loopback/CGNAT
ditolak, redirect tidak diikuti.

```go
sender := webhook.NewSender(10*time.Second, false) // false = mode produksi
sender.Header = "X-Signature"                       // opsional

resp, err := sender.Send(ctx, targetURL, secret, map[string]any{
    "transaction_id": id,
    "amount":         50137,
    "status":         "paid",
})
defer resp.Body.Close()
```

Cek URL saat merchant mendaftarkannya (gagal cepat sebelum disimpan):

```go
err := webhook.ValidateURL(ctx, inputURL, false)
```

**Terima** — verifikasi tanda tangan di sisi penerima, perbandingan constant-time:

```go
body, _ := io.ReadAll(r.Body)
if !webhook.Verify(secret, r.Header.Get("X-Signature"), body) {
    http.Error(w, "signature tidak cocok", http.StatusUnauthorized)
    return
}
```

> `allowPrivate = true` mematikan seluruh penjagaan SSRF dan mengizinkan redirect.
> Pakai hanya untuk tes ke localhost saat dev.

---

## 2. CLI

Untuk dipakai dari bahasa lain lewat exec, atau langsung dari shell saat debug.

```bash
go install github.com/saquone/qris/cmd/qris@latest
```

```bash
$ qris to-dynamic 50137 "000201010211266500..."
00020101021226650014COM.GO-JEK.WWW...5405501375802ID...63041B02

$ qris validate "000201010211..." && echo valid    # exit 0 = valid
$ qris merchant "000201010211..."
TOKO DEMO

$ qris decode qris-toko.png
000201010211266500...

$ echo "DANA: Anda menerima Rp 50.137 dari BUDI" | qris parse 'menerima Rp ?([0-9.,]+)'
50137
```

Payload kosong dibaca dari stdin, jadi bisa dirantai:
`qris decode foto.png | qris validate`

---

## 3. HTTP API

Stateless, tanpa database, tanpa autentikasi — jalankan di jaringan internal atau
taruh di belakang gateway-mu sendiri.

```bash
go install github.com/saquone/qris/cmd/qris-server@latest
qris-server -addr :8080
```

Buka **http://localhost:8080/docs** untuk Swagger UI — semua endpoint bisa dicoba
langsung dari browser.

| Endpoint | Request | Response |
|---|---|---|
| `GET /health` | — | `{"status":"ok"}` |
| `GET /docs` | — | Swagger UI |
| `GET /openapi.yaml` | — | Spec OpenAPI 3.1 |
| `POST /to-dynamic` | `{"payload":"...","amount":50137}` | `{"payload":"..."}` |
| `POST /validate` | `{"payload":"..."}` | `{"valid":true,"error":null}` |
| `POST /merchant` | `{"payload":"..."}` | `{"name":"TOKO DEMO"}` |
| `POST /decode` | bytes gambar mentah (PNG/JPEG) | `{"payload":"..."}` |
| `POST /parse` | `{"patterns":["..."],"text":"..."}` | `{"amount":50137}` |
| `POST /notification` | payload aplikasi listener Android | `{"amount":50137,"matched":true,...}` |
| `GET /notifications` | `?limit=50` | notifikasi tersimpan, terbaru dulu |

Gagal → HTTP 400 + `{"error":"pesan"}`. `POST /validate` selalu balik 200; hasilnya
ada di field `valid`.

Spec-nya di-`embed` ke binary (`cmd/qris-server/openapi.yaml`), jadi bisa dipakai
untuk generate client di bahasa lain:

```bash
curl -sO http://localhost:8080/openapi.yaml
```

Halaman `/docs` menarik aset Swagger UI dari CDN (butuh internet). `/openapi.yaml`
sendiri tetap jalan offline.

```bash
$ curl -s localhost:8080/to-dynamic -d '{"payload":"000201010211...","amount":50137}'
{"payload":"00020101021226650014COM.GO-JEK.WWW..."}

$ curl -s localhost:8080/decode --data-binary @qris-toko.png
{"payload":"000201010211266500..."}
```

API ini tidak *mengirim* webhook — tidak ada state, jadi tidak ada event untuk dikirim.
Untuk mengirim, pakai paket `webhook` dari aplikasimu sendiri.

### Pasangan langsung dengan aplikasi Android

`POST /notification` menerima apa adanya payload dari
[android-notification-listener](https://github.com/Saquone/android-notification-listener)
— **tanpa satu baris kode pun yang perlu kamu tulis**:

```bash
cat > patterns.txt <<'EOF'
(?i)Rp\s?([0-9.,]+)\s*diterima
(?i)menerima Rp ?([0-9.,]+)
EOF

qris-server -secret whsec_rahasia -patterns patterns.txt
```

Isi URL `http://<ip-servermu>:8080/notification` dan secret yang sama di aplikasi Android,
lalu tiap notifikasi masuk dijawab begini:

```json
{"amount":50137,"matched":true,"package_name":"id.dana","posted_at":1765432100000}
```

Endpoint hanya aktif kalau `-patterns` diberikan. Dengan `-secret`, header `X-Signature`
wajib cocok (`401` kalau tidak). Pola dicoba berurutan, jadi baris lama tetap jadi fallback
saat format teks bank berubah.

Notifikasi yang masuk **disimpan ke SQLite** (`-db`, default `qris.db`) dan bisa dibaca ulang:

```bash
curl 'http://localhost:8080/notifications?limit=20'
```

Duplikat diabaikan lewat unique index `(package_name, text, posted_at)` — aplikasi Android
mengirim ulang yang gagal dan menyapu ulang notification shade tiap rebind. Berkasnya SQLite
biasa, jadi bisa dibuka dengan `sqlite3` atau tool apa pun. `-db ""` mematikan penyimpanan.

Drivernya [modernc.org/sqlite](https://modernc.org/sqlite) — Go murni, **tanpa cgo**, jadi
`go install` jalan tanpa toolchain C dan cross-compile tetap mudah. Harganya: binary
`qris-server` naik dari ~10 MB ke ~16 MB. Library `qris` sendiri tidak terpengaruh —
dependensi ini hanya dipakai `cmd/qris-server`.

Notifikasi yang nominalnya tidak terbaca (promo, cashback) dijawab **200** dengan
`matched: false`, bukan error — kalau dijawab error, aplikasi Android akan mengirim ulang
selamanya.

Ini sengaja berhenti di "ekstrak nominal". Mencocokkannya dengan transaksi, menyimpan, dan
menagih adalah aplikasimu — lihat [Batas](#batas).

---

## Batas

Semuanya tanpa state: data masuk, data keluar. Konfirmasi pembayaran otomatis
(memasangkan notifikasi ke transaksi pending), pairing device, dan sweeper
transaksi kedaluwarsa butuh database, jadi tidak ada di sini — `notif`, `uniq`,
dan `webhook` cuma memberi bahan mentahnya.

## Lisensi

[MIT](LICENSE). `make test` sebelum kirim PR.
