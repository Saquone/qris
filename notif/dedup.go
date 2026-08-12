package notif

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

const DefaultWindow = time.Minute

// DedupKey membangun kunci stabil untuk satu notifikasi: sha256 dari
// scope|raw|amount|waktu-dibulatkan-ke-window. scope memisahkan ruang kunci antar
// pemilik (mis. user ID). Simpan di kolom ber-unique-index; insert yang tertolak
// = notifikasi duplikat. window <= 0 dianggap DefaultWindow.
func DedupKey(scope, raw string, amount int64, at time.Time, window time.Duration) string {
	if window <= 0 {
		window = DefaultWindow
	}
	rounded := at.Unix() / int64(window.Seconds())
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%d|%d", scope, raw, amount, rounded)))
	return hex.EncodeToString(sum[:])
}
