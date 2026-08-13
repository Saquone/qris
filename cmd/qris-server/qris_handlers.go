package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/saquone/qris"
	"github.com/saquone/qris/qrimage"
	"github.com/saquone/qris/uniq"
)

// registerQRIS memasang alur lengkap: unggah QRIS statis → buat tagihan bernominal →
// pembayaran diverifikasi otomatis dari notifikasi yang masuk.
func (s *server) registerQRIS(mux *http.ServeMux) {
	// Unggah gambar QRIS statis. Body = bytes gambar mentah; berkasnya disimpan di -qris-dir
	// supaya bisa dilihat lagi, payload teksnya masuk SQLite.
	mux.HandleFunc("POST /qris", func(w http.ResponseWriter, r *http.Request) {
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
		if err := qris.ValidateStatic(payload); err != nil {
			fail(w, err)
			return
		}
		name := qris.ExtractMerchantName(payload)

		file := fmt.Sprintf("qris-%d.png", time.Now().UnixMilli())
		if err := os.WriteFile(filepath.Join(s.qrisDir, file), data, 0o644); err != nil {
			fail(w, err)
			return
		}
		id, err := s.store.SaveMerchant(Merchant{Name: name, Payload: payload, ImageFile: file})
		if err != nil {
			fail(w, err)
			return
		}
		write(w, http.StatusOK, map[string]any{"id": id, "name": name, "image_file": file, "payload": payload})
	})

	mux.HandleFunc("GET /qris", func(w http.ResponseWriter, r *http.Request) {
		list, err := s.store.Merchants()
		if err != nil {
			fail(w, err)
			return
		}
		write(w, http.StatusOK, map[string]any{"merchants": list})
	})

	// Buat tagihan: QRIS statis + nominal → QRIS dinamis. Kode unik 1..999 ditambahkan supaya
	// dua tagihan tidak pernah bernominal sama — tanpa itu notifikasi masuk tidak bisa dipetakan
	// ke satu tagihan tertentu.
	mux.HandleFunc("POST /charges", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			MerchantID int64 `json:"merchant_id"`
			Amount     int64 `json:"amount"`
			UniqueMin  *int  `json:"unique_min"`
			UniqueMax  *int  `json:"unique_max"`
			ExpiresIn  int   `json:"expires_in_seconds"`
		}
		if !read(w, r, &req) {
			return
		}
		m, err := s.store.Merchant(req.MerchantID)
		if err != nil {
			fail(w, err)
			return
		}
		min, max := 1, 999
		if req.UniqueMin != nil {
			min = *req.UniqueMin
		}
		if req.UniqueMax != nil {
			max = *req.UniqueMax
		}
		taken, err := s.store.TakenAmounts(m.ID, req.Amount)
		if err != nil {
			fail(w, err)
			return
		}
		code, err := uniq.Pick(req.Amount, min, max, taken)
		if err != nil {
			fail(w, err)
			return
		}
		final := req.Amount + int64(code)
		payload, err := qris.ToDynamic(m.Payload, final)
		if err != nil {
			fail(w, err)
			return
		}
		ttl := req.ExpiresIn
		if ttl <= 0 {
			ttl = 15 * 60
		}
		now := time.Now()
		c := Charge{
			MerchantID: m.ID, BaseAmount: req.Amount, UniqueCode: code, Amount: final, Payload: payload,
			CreatedAt: now.UnixMilli(), ExpiresAt: now.Add(time.Duration(ttl) * time.Second).UnixMilli(),
		}
		id, err := s.store.SaveCharge(c)
		if err != nil {
			fail(w, err)
			return
		}
		c.ID, c.Status = id, ChargePending
		write(w, http.StatusOK, c)
	})

	mux.HandleFunc("GET /charges", func(w http.ResponseWriter, r *http.Request) {
		list, err := s.store.Charges(limitParam(r, 50, 500))
		if err != nil {
			fail(w, err)
			return
		}
		write(w, http.StatusOK, map[string]any{"charges": list})
	})

	mux.HandleFunc("GET /charges/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			fail(w, err)
			return
		}
		c, err := s.store.Charge(id)
		if err != nil {
			fail(w, err)
			return
		}
		write(w, http.StatusOK, c)
	})
}

func limitParam(r *http.Request, def, max int) int {
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= max {
			return n
		}
	}
	return def
}

func write(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(body)
}

func fail(w http.ResponseWriter, err error) {
	write(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
}

func read(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(io.LimitReader(r.Body, maxJSON)).Decode(dst); err != nil {
		fail(w, err)
		return false
	}
	return true
}
