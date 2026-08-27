package event

import (
	"crypto/rand"
	"encoding/hex"
)

// newEventID membuat UUID v4 sebagai string kanonik 36 karakter.
//
// Ditulis tangan, bukan lewat pustaka UUID, karena modul ini tidak punya satu pun
// dan menambah dependensi eksternal untuk 16 byte acak akan melanggar §16.4.
// crypto/rand adalah sumbernya: id event muncul di kabel klien dan menjadi kunci
// dedup AlertDedup, jadi ia harus tak dapat ditebak, bukan hanya unik.
//
// crypto/rand.Read pada Go 1.24 tidak pernah gagal (ia panic di dalam bila entropi
// sistem tak tersedia), jadi tidak ada cabang galat di sini yang dapat diuji.
func newEventID() string {
	var b [16]byte
	rand.Read(b[:]) //nolint:errcheck // lihat komentar di atas
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	var out [36]byte
	hex.Encode(out[0:8], b[0:4])
	out[8] = '-'
	hex.Encode(out[9:13], b[4:6])
	out[13] = '-'
	hex.Encode(out[14:18], b[6:8])
	out[18] = '-'
	hex.Encode(out[19:23], b[8:10])
	out[23] = '-'
	hex.Encode(out[24:36], b[10:16])
	return string(out[:])
}
