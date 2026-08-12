// Package webhook mengirim dan memverifikasi webhook bertanda tangan HMAC-SHA256.
package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"
)

const DefaultHeader = "X-Signature"

var (
	ErrScheme    = errors.New("URL webhook harus diawali http:// atau https://")
	ErrHTTPS     = errors.New("URL webhook harus menggunakan HTTPS")
	ErrHostname  = errors.New("URL webhook tidak memiliki hostname yang valid")
	ErrResolve   = errors.New("URL webhook tidak dapat di-resolve")
	ErrPrivateIP = errors.New("URL webhook tidak boleh menunjuk ke jaringan internal atau lokal")
)

// cgnat = 100.64.0.0/10 (RFC 6598), dipakai sebagai jaringan internal pod/overlay
// di banyak CNI dan Tailscale; net.IP.IsPrivate tidak mencakupnya.
var _, cgnat, _ = net.ParseCIDR("100.64.0.0/10")

func isPrivateIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || cgnat.Contains(ip)
}

// Sign menghitung tanda tangan HMAC-SHA256 atas body, dalam hex.
func Sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// Verify memeriksa tanda tangan dengan perbandingan constant-time. Dipakai di sisi
// penerima webhook.
func Verify(secret, signature string, body []byte) bool {
	return hmac.Equal([]byte(signature), []byte(Sign(secret, body)))
}

// Sender mengirim webhook lewat http.Client yang dikeraskan terhadap SSRF.
type Sender struct {
	Header string // default DefaultHeader
	client *http.Client
}

// NewSender membuat pengirim webhook. allowPrivate=true melewati semua penjagaan
// SSRF dan mengizinkan redirect — hanya untuk dev/localhost, jangan di produksi.
func NewSender(timeout time.Duration, allowPrivate bool) *Sender {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			dialer := &net.Dialer{Timeout: 5 * time.Second}
			if allowPrivate {
				return dialer.DialContext(ctx, network, addr)
			}
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				host, port = addr, ""
			}
			ips, err := lookup(ctx, host)
			if err != nil {
				return nil, err
			}
			for _, ip := range ips {
				if isPrivateIP(ip) {
					return nil, fmt.Errorf("target %s private range (DNS rebinding?)", ip)
				}
			}
			// Dial IP yang baru divalidasi, bukan hostname: lookup kedua di dalam
			// net.Dialer bisa mengembalikan IP (privat) yang berbeda.
			return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
		},
	}
	return &Sender{
		Header: DefaultHeader,
		client: &http.Client{
			Timeout:   timeout,
			Transport: transport,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				if allowPrivate {
					return nil
				}
				return http.ErrUseLastResponse
			},
		},
	}
}

// Send mem-marshal payload jadi JSON, menandatanganinya, lalu POST ke target.
// Pemanggil bertanggung jawab menutup resp.Body.
func (s *Sender) Send(ctx context.Context, target, secret string, payload any) (*http.Response, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(s.Header, Sign(secret, body))
	return s.client.Do(req)
}

// ValidateURL memeriksa URL saat didaftarkan — gagal cepat sebelum disimpan.
// Pemeriksaan otoritatifnya tetap di Sender saat connect, karena DNS bisa berubah.
func ValidateURL(ctx context.Context, rawURL string, allowPrivate bool) error {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return ErrScheme
	}
	if !allowPrivate && u.Scheme != "https" {
		return ErrHTTPS
	}
	if u.Hostname() == "" {
		return ErrHostname
	}
	if allowPrivate {
		return nil
	}
	ips, err := lookup(ctx, u.Hostname())
	if err != nil {
		return ErrResolve
	}
	for _, ip := range ips {
		if isPrivateIP(ip) {
			return ErrPrivateIP
		}
	}
	return nil
}

func lookup(ctx context.Context, host string) ([]net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		return []net.IP{ip}, nil
	}
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("gagal me-resolve host %s: %w", host, err)
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("host %s tidak menghasilkan alamat IP", host)
	}
	ips := make([]net.IP, 0, len(addrs))
	for _, a := range addrs {
		ips = append(ips, a.IP)
	}
	return ips, nil
}
