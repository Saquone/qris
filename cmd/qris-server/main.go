// Command qris-server mengekspos library ini sebagai HTTP API JSON — stateless,
// tanpa database, untuk dipakai dari bahasa apa pun.
package main

import (
	_ "embed"
	"encoding/json"
	"flag"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/saquone/qris"
	"github.com/saquone/qris/catalog"
	"github.com/saquone/qris/notif"
	"github.com/saquone/qris/qrimage"
	"github.com/saquone/qris/webhook"
)

const (
	maxJSON  = 1 << 20 // 1 MB
	maxImage = 5 << 20 // 5 MB
)

//go:embed openapi.yaml
var openapiSpec []byte

// Swagger UI diambil dari CDN (versi dipin) supaya repo tidak menyimpan ~3 MB aset.
// Butuh internet saat membuka /docs; /openapi.yaml sendiri tetap jalan offline.
const docsHTML = `<!doctype html>
<html><head><meta charset="utf-8"><title>qris-server API</title>
<link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5.32.12/swagger-ui.css"></head>
<body><div id="ui"></div>
<script src="https://unpkg.com/swagger-ui-dist@5.32.12/swagger-ui-bundle.js"></script>
<script>SwaggerUIBundle({url: "openapi.yaml", dom_id: "#ui"})</script>
</body></html>`

func main() {
	addr := flag.String("addr", ":8080", "alamat listen")
	secret := flag.String("secret", "", "secret HMAC untuk /notification (kosong = tanda tangan tidak diperiksa)")
	patternsFile := flag.String("patterns", "", "berkas pola regex nominal, satu per baris (default: katalog bawaan)")
	dbPath := flag.String("db", "qris.db", "berkas SQLite penyimpan notifikasi (kosong = tidak disimpan)")
	catalogFile := flag.String("catalog", "", "berkas katalog gateway JSON (default: katalog bawaan)")
	flag.Parse()

	var store *Store
	if *dbPath != "" {
		s, err := OpenStore(*dbPath)
		if err != nil {
			log.Fatalf("gagal membuka %s: %v", *dbPath, err)
		}
		defer s.Close()
		store = s
	}

	catalogJSON := catalog.Raw()
	if *catalogFile != "" {
		b, err := os.ReadFile(*catalogFile)
		if err != nil {
			log.Fatalf("gagal membaca %s: %v", *catalogFile, err)
		}
		var check []catalog.Gateway
		if err := json.Unmarshal(b, &check); err != nil || len(check) == 0 {
			log.Fatalf("katalog %s tidak valid: %v", *catalogFile, err)
		}
		catalogJSON = b
		log.Printf("katalog dari %s (%d gateway)", *catalogFile, len(check))
	}

	// Tanpa -patterns, pakai katalog bawaan — /notification aktif sejak awal tanpa konfigurasi.
	var parser *notif.Parser
	var err error
	if *patternsFile != "" {
		raw, e := os.ReadFile(*patternsFile)
		if e != nil {
			log.Fatalf("gagal membaca %s: %v", *patternsFile, e)
		}
		if parser, err = notif.NewFromTemplate(string(raw)); err != nil {
			log.Fatalf("pola di %s tidak ada yang valid: %v", *patternsFile, err)
		}
		log.Printf("pola nominal dari %s", *patternsFile)
	} else if parser, err = catalog.Parser(); err != nil {
		log.Fatalf("katalog bawaan rusak: %v", err)
	}
	if *secret == "" {
		log.Print("PERINGATAN: -secret kosong, tanda tangan /notification TIDAK diperiksa")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		write(w, http.StatusOK, map[string]any{"status": "ok"})
	})

	// Katalog gateway: aplikasi Android mengambil daftar aplikasi yang didukung + pola parsernya
	// dari sini, lalu menyimpannya untuk dipakai offline. -catalog menimpanya dengan berkas
	// sendiri, buat yang banknya belum ada di katalog bawaan.
	mux.HandleFunc("GET /gateways", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(catalogJSON)
	})

	mux.HandleFunc("GET /openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		w.Write(openapiSpec)
	})
	mux.HandleFunc("GET /docs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(docsHTML))
	})

	mux.HandleFunc("POST /to-dynamic", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Payload string `json:"payload"`
			Amount  int64  `json:"amount"`
		}
		if !read(w, r, &req) {
			return
		}
		out, err := qris.ToDynamic(req.Payload, req.Amount)
		if err != nil {
			fail(w, err)
			return
		}
		write(w, http.StatusOK, map[string]any{"payload": out})
	})

	mux.HandleFunc("POST /validate", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Payload string `json:"payload"`
		}
		if !read(w, r, &req) {
			return
		}
		res := map[string]any{"valid": true, "error": nil}
		if err := qris.ValidateStatic(req.Payload); err != nil {
			res["valid"], res["error"] = false, err.Error()
		}
		write(w, http.StatusOK, res)
	})

	mux.HandleFunc("POST /merchant", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Payload string `json:"payload"`
		}
		if !read(w, r, &req) {
			return
		}
		write(w, http.StatusOK, map[string]any{"name": qris.ExtractMerchantName(req.Payload)})
	})

	// Body = bytes gambar mentah (PNG/JPEG), bukan JSON.
	mux.HandleFunc("POST /decode", func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(io.LimitReader(r.Body, maxImage))
		if err != nil {
			fail(w, err)
			return
		}
		payload, err := qrimage.Decode(data)
		if err != nil {
			fail(w, err)
			return
		}
		write(w, http.StatusOK, map[string]any{"payload": payload})
	})

	mux.HandleFunc("POST /parse", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Patterns []string `json:"patterns"`
			Text     string   `json:"text"`
		}
		if !read(w, r, &req) {
			return
		}
		p, err := notif.New(req.Patterns)
		if err != nil {
			fail(w, err)
			return
		}
		amount, err := p.ParseAmount(req.Text)
		if err != nil {
			fail(w, err)
			return
		}
		write(w, http.StatusOK, map[string]any{"amount": amount})
	})

	// Menerima langsung payload dari github.com/Saquone/android-notification-listener.
	mux.HandleFunc("POST /notification", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, maxJSON))
		if err != nil {
			fail(w, err)
			return
		}
		if *secret != "" && !webhook.Verify(*secret, r.Header.Get(webhook.DefaultHeader), body) {
			write(w, http.StatusUnauthorized, map[string]any{"error": "tanda tangan tidak cocok"})
			return
		}
		var req struct {
			PackageName string `json:"package_name"`
			Title       string `json:"title"`
			Text        string `json:"text"`
			PostedAt    int64  `json:"posted_at"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			fail(w, err)
			return
		}
		res := map[string]any{
			"package_name": req.PackageName,
			"posted_at":    req.PostedAt,
			"matched":      false,
			"amount":       nil,
		}
		// Nominal tak terbaca BUKAN error: notifikasi promo/cashback ikut terkirim dan
		// harus dijawab 2xx, kalau tidak aplikasi mengirim ulang selamanya.
		var amount *int64
		if a, err := parser.ParseAmount(req.Title + " " + req.Text); err == nil {
			amount = &a
			res["matched"], res["amount"] = true, a
		}
		if store != nil {
			n := Notification{
				PackageName: req.PackageName, Title: req.Title, Text: req.Text,
				PostedAt: req.PostedAt, Amount: amount,
			}
			if err := store.Save(n); err != nil {
				log.Printf("gagal menyimpan notifikasi: %v", err)
			}
		}
		write(w, http.StatusOK, res)
	})

	if store != nil {
		mux.HandleFunc("GET /notifications", func(w http.ResponseWriter, r *http.Request) {
			limit := 50
			if v := r.URL.Query().Get("limit"); v != "" {
				if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 500 {
					limit = n
				}
			}
			list, err := store.List(limit)
			if err != nil {
				fail(w, err)
				return
			}
			write(w, http.StatusOK, map[string]any{"notifications": list})
		})
		log.Printf("notifikasi disimpan di %s", *dbPath)
	}

	log.Printf("qris-server listen di %s — dokumentasi di %s/docs", *addr, *addr)
	log.Fatal(http.ListenAndServe(*addr, mux))
}

func read(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(io.LimitReader(r.Body, maxJSON)).Decode(dst); err != nil {
		fail(w, err)
		return false
	}
	return true
}

func write(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(body)
}

func fail(w http.ResponseWriter, err error) {
	write(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
}
