# saquone/qris

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Self-Host Ready](https://img.shields.io/badge/Self--Host-Ready-orange.svg)](#)
[![Go Reference](https://pkg.go.dev/badge/github.com/saquone/qris.svg)](https://pkg.go.dev/github.com/saquone/qris)

Library Go **Self-Hosted** open-source dan server HTTP untuk **QRIS**: mengubah QRIS statis menjadi QRIS dinamis bernominal, validasi payload EMVCo, membaca QR dari gambar, mengekstrak nominal dari notifikasi bank/e-wallet (DANA Bisnis, BRI, GoPay, Grab, dll), serta mengirim webhook bertanda tangan HMAC-SHA256.

Diekstrak dari core payment engine [saquone](https://github.com/Saquone). **100% Self-Hosted. Tanpa autentikasi wajib, tanpa database rumit, tanpa API key. Lisensi MIT.**

Dapat digunakan melalui **3 cara**:
1. **Library Go** (zero-dependency core package)
2. **CLI Command Line Tool** (`bin/qris`)
3. **HTTP API Server** (`bin/qris-server`)

---

## 1. Library Go

```bash
go get github.com/saquone/qris
```

*Membutuhkan Go 1.22+.*

### ⚡ Konversi & Validasi QRIS

```go
import "github.com/saquone/qris"

// Tolak payload yang bukan QRIS statis IDR (CRC salah, tag kurang, sudah dinamis, non-IDR)
if err := qris.ValidateStatic(statisPayload); err != nil {
    return err
}

// Konversi QRIS Statis -> Dinamis:
// Tag 01: 11 (statis) -> 12 (dinamis), menyisipkan Tag 54 (nominal), dan menghitung ulang CRC16 (Tag 63).
dinamisPayload, err := qris.ToDynamic(statisPayload, 50137)

namaMerchant := qris.ExtractMerchantName(statisPayload) // "TOKO DEMO"

items, _ := qris.Parse(statisPayload)
mcc, _ := qris.Get(items, "52")
```

*Identitas merchant dan rekening tujuan dana **tidak diubah** — hanya menambahkan nominal, sehingga dana tetap masuk langsung ke rekening merchant.*

---

### 📷 Dekode QR dari Gambar (`qris/qrimage`)

```go
import "github.com/saquone/qris/qrimage"

// Membaca payload QRIS dari file PNG atau JPEG
payload, err := qrimage.Decode(bytesPNG)
```

`qrimage.MaxPixels` (default ~25 MP) mencegah pembacaan file berukuran raksasa yang berpotensi menghabiskan memori server.

---

### 📩 Ekstraksi Nominal Notifikasi (`qris/notif`)

```go
import "github.com/saquone/qris/notif"

p, _ := notif.New([]string{
    `(?i)Rp\s?([0-9.,]+)\s*diterima`, // Pola utama (misal: DANA Bisnis)
    `(?i)menerima Rp ?([0-9.,]+)`,     // Fallback
})

// Ekstrak nominal dari teks notifikasi
amount, err := p.ParseAmount("Pembayaran Masuk: Rp1.426 diterima DANA Bisnis.") // 1426
```

Gunakan `DedupKey` untuk mencegah pemrosesan ulang notifikasi duplikat dari Android:

```go
key := notif.DedupKey(userID, rawText, amount, capturedAt, time.Minute)
```

---

### 🔢 Kode Unik Anti-Tabrakan (`qris/uniq`)

Untuk membedakan transaksi pending dengan nominal dasar yang sama (misal dua pelanggan membayar Rp50.000 bersamaan), tambahkan kode unik 1–999:

```go
import "github.com/saquone/qris/uniq"

// taken = daftar nominal transaksi pending yang sedang menunggu pembayaran
kode, err := uniq.Pick(50000, 1, 999, []int64{50137, 50250})
nominalFinal := 50000 + int64(kode) // 50000 + 426 = 50426
```

---

### 🔒 Webhook Engine (`qris/webhook`)

* **Pengirim (Sender) dengan Anti-SSRF:** Dikeraskan terhadap serangan DNS Rebinding, menolak IP privat/loopback/CGNAT, dan tidak mengikuti redirect.

```go
import "github.com/saquone/qris/webhook"

sender := webhook.NewSender(10*time.Second, false) // false = mode produksi
sender.Header = "X-Signature"

resp, err := sender.Send(ctx, targetURL, secretKey, map[string]any{
    "transaction_id": 3,
    "amount":         1426,
    "status":         "paid",
})
```

* **Penerima (Receiver Verification):** Verifikasi tanda tangan HMAC-SHA256 secara constant-time:

```go
body, _ := io.ReadAll(r.Body)
if !webhook.Verify(secretKey, r.Header.Get("X-Signature"), body) {
    http.Error(w, "tanda tangan tidak cocok", http.StatusUnauthorized)
    return
}
```

---

## 2. CLI Command Line Tool

Dapat digunakan dari shell, script, atau exec bahasa pemrograman lain.

```bash
go install github.com/saquone/qris/cmd/qris@latest
```

```bash
# Konversi QRIS statis ke dinamis
qris to-dynamic 50137 "0002010102112665..."

# Validasi payload QRIS statis
qris validate "000201010211..."

# Ekstrak nama merchant
qris merchant "000201010211..."

# Dekode gambar QRIS
qris decode qris-toko.png

# Pipe / Chaining
qris decode qris-toko.png | qris validate
```

---

## 3. HTTP API Server (`qris-server`)

Server HTTP siap pakai yang menyediakan endpoint konversi QRIS, pencocokan notifikasi otomatis (*Matching Engine*), dan penyedia katalog gateway.

```bash
go install github.com/saquone/qris/cmd/qris-server@latest
qris-server -addr :8080
```

Buka **`http://localhost:8080/docs`** di browser untuk **Swagger UI Interactive Documentation**. Spec OpenAPI 3.1 mentah tersedia di `/openapi.yaml`.

### 📋 Daftar Endpoint HTTP

| Endpoint | Method | Deskripsi |
|---|---|---|
| `/health` | `GET` | Cek status server (`{"status":"ok"}`) |
| `/docs` | `GET` | Interactive Swagger UI Documentation |
| `/openapi.yaml` | `GET` | Spesifikasi OpenAPI 3.1 |
| `/to-dynamic` | `POST` | Konversi payload QRIS statis → dinamis bernominal |
| `/validate` | `POST` | Validasi kelayakan payload QRIS statis IDR |
| `/merchant` | `POST` | Mengambil nama merchant (Tag 59) dari payload |
| `/decode` | `POST` | Dekode QRIS dari binary gambar (PNG/JPEG) |
| `/qris` | `POST / GET` | Unggah & kelola QRIS statis merchant |
| `/charges` | `POST / GET` | Buat tagihan baru (QRIS dinamis + kode unik) |
| `/charges/{id}` | `GET` | Cek status tagihan (`pending` / `paid` / `expired`) |
| `/gateways` | `GET` | Katalog gateway yang didukung (diambil oleh Android app) |
| `/notification` | `POST` | Terima webhook notifikasi dari [android-notification-listener](https://github.com/Saquone/android-notification-listener) |
| `/notifications` | `GET` | Riwayat notifikasi yang tersimpan di SQLite DB (`qris.db`) |
| `/parse` | `POST` | Ekstrak nominal dari teks notifikasi mentah |

---

### 🔄 Integrasi & Matching Engine Otomatis

`qris-server` dilengkapi dengan **Matching Engine berbasis SQLite** yang secara otomatis mencocokkan notifikasi masuk dari HP Android ke tagihan pending:

```bash
# 1. Jalankan server dengan secret HMAC
qris-server -secret secret_rahasia_kamu

# 2. Unggah gambar QRIS statis toko
curl -X POST http://localhost:8080/qris --data-binary @qris-toko.png

# 3. Buat tagihan baru sebesar Rp1.000
curl -X POST http://localhost:8080/charges -d '{"amount":1000}'
# → {"id":3,"amount":1426,"payload":"000201010212...","status":"pending"}

# 4. Saat notifikasi pembayar DANA Bisnis Rp1.426 masuk dari Android Listener:
#    Matching Engine secara otomatis mengubah status tagihan menjadi lunas ("paid")!
curl http://localhost:8080/charges/3
# → {"id":3,"amount":1426,"status":"paid","paid_at":1787117996120}
```

---

## 📄 Lisensi

Proyek ini dirilis di bawah lisensi **[MIT License](LICENSE)**.
