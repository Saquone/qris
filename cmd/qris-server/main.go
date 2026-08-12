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

	"github.com/saquone/qris"
	"github.com/saquone/qris/notif"
	"github.com/saquone/qris/qrimage"
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
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		write(w, http.StatusOK, map[string]any{"status": "ok"})
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
