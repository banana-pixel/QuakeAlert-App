//go:build ignore

// Helper manual: dijalankan lewat `go run scripts/setup_nodes.go`.
// Lihat catatan tag ignore di simulate_trigger.go.

package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/banana-pixel/quakealert/server/internal/crypto"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	masterKeyHex := os.Getenv("MASTER_KEY_HEX")
	if masterKeyHex == "" {
		log.Fatal("MASTER_KEY_HEX wajib di-set")
	}

	mh := strings.TrimSpace(masterKeyHex)
	if len(mh) != 64 {
		log.Fatalf("MASTER_KEY_HEX harus 64 karakter hex, dapat %d", len(mh))
	}
	var masterKey [32]byte
	decoded, err := hex.DecodeString(mh)
	if err != nil {
		log.Fatalf("MASTER_KEY_HEX tidak valid hex: %v", err)
	}
	copy(masterKey[:], decoded)

	log.Printf("Master key (first 8 bytes): %x", masterKey[:8])

	cf, err := crypto.New(masterKey)
	if err != nil {
		log.Fatalf("gagal membuat cipher: %v", err)
	}
	log.Printf("Cipher created successfully")

	dbURL := "postgres://quakealert:devpassword@localhost:5432/quakealert?sslmode=disable"
	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		log.Fatalf("gagal koneksi ke database: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(context.Background()); err != nil {
		log.Fatalf("ping database gagal: %v", err)
	}
	log.Printf("Koneksi database berhasil")

	type nodeDef struct {
		stationID string
		lat, lon  float64
		name      string
	}
	nodes := [3]nodeDef{
		{"NODE-AAAAAAAA", -6.91, 107.61, "Bandung Station 1"},
		{"NODE-BBBBBBBB", -6.92, 107.62, "Bandung Station 2"},
		{"NODE-CCCCCCCC", -6.90, 107.60, "Bandung Station 3"},
	}

	secretsPlain := [3]string{
		"secret_aaa_bandung_1",
		"secret_bbb_bandung_2",
		"secret_ccc_bandung_3",
	}

	log.Println("Mengenkripsi dan menyisipkan 3 node IoT...")
	log.Println("")

	for i := 0; i < 3; i++ {
		secretBytes := []byte(secretsPlain[i])
		ciphertext, nonce, err := cf.Encrypt(secretBytes)
		if err != nil {
			log.Fatalf("gagal enkripsi secret node %d (%s): %v", i+1, nodes[i].stationID, err)
		}

		log.Printf("Node %d: %s", i+1, nodes[i].stationID)
		log.Printf("  plaintext: %s", secretsPlain[i])
		log.Printf("  ciphertext (hex): %x", ciphertext)
		log.Printf("  nonce (hex):      %x", nonce)

		_, err = pool.Exec(context.Background(),
			`INSERT INTO iot_nodes (
				station_id, sensor_model, location_name, location,
				secret_key_enc, secret_key_nonce, key_version, is_active
			) VALUES (
				$1, $2, $3, ST_SetSRID(ST_MakePoint($4, $5), 4326)::geography,
				$5, $6, $7, $8
			)`,
			nodes[i].stationID,
			"MPU 6050",
			nodes[i].name,
			nodes[i].lon,
			nodes[i].lat,
			ciphertext,
			nonce,
			1,
			true,
		)
		if err != nil {
			log.Fatalf("Gagal menyisipkan node %s: %v", nodes[i].stationID, err)
		}
		log.Printf("  OK Node disimpan ke database")
		log.Println("")
	}

	log.Println("=== 3 Node IoT berhasil disimpan ===")
	log.Println("")
	log.Println("Secrets plaintext (untuk digunakan di simulate_trigger.go):")
	for i := 0; i < 3; i++ {
		fmt.Printf("  %s: %s\n", nodes[i].stationID, secretsPlain[i])
	}
	log.Println("")
	fmt.Printf("go run server/scripts/simulate_trigger.go \\\n")
	for i := 0; i < 3; i++ {
		fmt.Printf("  %s", secretsPlain[i])
		if i < 2 {
			fmt.Print(" \\")
		}
	}
	log.Println()
}
