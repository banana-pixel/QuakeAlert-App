//go:build ignore

// sim_setup_nodes.go — insert 3 sim nodes directly into dev DB.
// Bypasses provisioning API rate limit. Dev-only, never production.
//
// Usage:
//   go run scripts/sim_setup_nodes.go
//
// Env:
//   DATABASE_URL (default: postgres://quakealert:devpassword@localhost:5432/quakealert?sslmode=disable)
//   MASTER_KEY_HEX (default: 0000000000000000000000000000000000000000000000000000000000000001)
//
// Prints plaintext secrets to stdout for use by sim_confirm_scenario.go.
// Nodes are inserted with is_active=true and verified=true.
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

var simNodeDefs = []struct {
	StationID string
	Lat, Lon  float64
	Name      string
	Secret    string // plaintext — must match sim_confirm_scenario.go
}{
	{"NODE-53494D41", -6.900, 107.600, "Sim Station Alpha", "sec_sim_alpha_checkpoint_3_1_aaaa"},
	{"NODE-53494D42", -6.855, 107.600, "Sim Station Bravo", "sec_sim_bravo_checkpoint_3_1_bbbb"},
	{"NODE-53494D43", -6.910, 107.650, "Sim Station Charlie", "sec_sim_charlie_checkpoint_3_1_cc"},
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

	for _, n := range simNodeDefs {
		ct, nonce, err := cf.Encrypt([]byte(n.Secret))
		if err != nil {
			fmt.Fprintf(os.Stderr, "encrypt %s: %v\n", n.StationID, err)
			os.Exit(1)
		}

		// Upsert: delete existing sim node first so re-runs are idempotent.
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
			n.Lon, n.Lat,
			ct, nonce,
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "insert %s: %v\n", n.StationID, err)
			os.Exit(1)
		}
		fmt.Printf("inserted %s\n", n.StationID)
	}

	// Print secrets for sim_confirm_scenario.go args.
	fmt.Println("---secrets---")
	for _, n := range simNodeDefs {
		fmt.Println(n.Secret)
	}
}
