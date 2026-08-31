//go:build ignore

// sim_setup_nodes_6.go — Checkpoint 3.2: insert 6 sim nodes for dual-event scenario.
//
// Cluster A (Bandung area): NODE-53494D41/42/43 — same as 3.1.
// Cluster B (Surabaya area): NODE-535542{41/42/43} — ~570 km from A.
// Separation between clusters >> AttachRadiusKm=50km → guaranteed separate events.
//
// Usage:
//
//	go run scripts/sim_setup_nodes_6.go
//
// Env:
//
//	DATABASE_URL  (default: postgres://quakealert:devpassword@localhost:5432/quakealert?sslmode=disable)
//	MASTER_KEY_HEX (default: 000...0001)
package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"github.com/banana-pixel/quakealert/server/internal/crypto"
	"github.com/jackc/pgx/v5/pgxpool"
)

// simNodeDefs6 — both clusters.
// Cluster A: Bandung (-6.9°, 107.6°) — nodes separated ≥5 km each.
// Cluster B: Surabaya (-7.25°, 112.75°) — nodes separated ≥5 km each.
// Inter-cluster distance ≈570 km >> AttachRadiusKm=50 → two separate events.
var simNodeDefs6 = []struct {
	StationID string
	Lat, Lon  float64
	Name      string
	Secret    string
}{
	// Cluster A — Bandung
	{"NODE-53494D41", -6.900, 107.600, "Sim-A Alpha Bandung", "sec_sim_alpha_checkpoint_3_1_aaaa"},
	{"NODE-53494D42", -6.855, 107.600, "Sim-A Bravo Bandung", "sec_sim_bravo_checkpoint_3_1_bbbb"},
	{"NODE-53494D43", -6.910, 107.650, "Sim-A Charlie Bandung", "sec_sim_charlie_checkpoint_3_1_cc"},
	// Cluster B — Surabaya
	// -7.250 ↔ -7.205: HaversineKm ≈ 5.01 km → independent
	// -7.250 ↔ -7.260/112.800: ≈ 5.5 km → independent
	{"NODE-53554241", -7.250, 112.750, "Sim-B Alpha Surabaya", "sec_sim_b_alpha_checkpoint_3_2_aa"},
	{"NODE-53554242", -7.205, 112.750, "Sim-B Bravo Surabaya", "sec_sim_b_bravo_checkpoint_3_2_bb"},
	{"NODE-53554243", -7.260, 112.800, "Sim-B Charlie Surabaya", "sec_sim_b_charlie_ckpt_3_2_cc"},
}

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://quakealert:devpassword@localhost:5432/quakealert?sslmode=disable"
	}
	masterKeyHex := os.Getenv("MASTER_KEY_HEX")
	if masterKeyHex == "" {
		masterKeyHex = "0000000000000000000000000000000000000000000000000000000000000001"
	}

	mh := strings.TrimSpace(masterKeyHex)
	decoded, err := hex.DecodeString(mh)
	if err != nil || len(decoded) != 32 {
		fmt.Fprintf(os.Stderr, "invalid MASTER_KEY_HEX: %v\n", err)
		os.Exit(1)
	}
	var masterKey [32]byte
	copy(masterKey[:], decoded)

	cf, err := crypto.New(masterKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "crypto.New: %v\n", err)
		os.Exit(1)
	}

	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pgxpool.New: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	for _, n := range simNodeDefs6 {
		ct, nonce, err := cf.Encrypt([]byte(n.Secret))
		if err != nil {
			fmt.Fprintf(os.Stderr, "encrypt %s: %v\n", n.StationID, err)
			os.Exit(1)
		}
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM iot_nodes WHERE station_id = $1`, n.StationID)
		_, err = pool.Exec(context.Background(), `
			INSERT INTO iot_nodes (
				station_id, sensor_model, location_name, location,
				secret_key_enc, secret_key_nonce, key_version,
				is_active, verified
			) VALUES (
				$1, $2, $3,
				ST_SetSRID(ST_MakePoint($4, $5), 4326)::geography,
				$6, $7, 1, true, true
			)`,
			n.StationID, "SIM-MPU6050", n.Name,
			n.Lon, n.Lat, ct, nonce,
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "insert %s: %v\n", n.StationID, err)
			os.Exit(1)
		}
		fmt.Printf("inserted %s\n", n.StationID)
	}
	fmt.Println("---secrets-a---")
	for _, n := range simNodeDefs6[:3] {
		fmt.Println(n.Secret)
	}
	fmt.Println("---secrets-b---")
	for _, n := range simNodeDefs6[3:] {
		fmt.Println(n.Secret)
	}
}
