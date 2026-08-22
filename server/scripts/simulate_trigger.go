//go:build ignore

// Helper manual: dijalankan lewat `go run scripts/simulate_trigger.go <secret...>`.
// Tag ignore dipasang karena file ini dan setup_nodes.go sama-sama `package main`
// dalam satu direktori, sehingga tanpa tag `go build ./...` gagal (main ganda).

package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

func main() {
	if len(os.Args) != 4 {
		fmt.Fprintln(os.Stderr, "Usage: go run simulate_trigger.go <secret1> <secret2> <secret3>")
		fmt.Fprintln(os.Stderr, "  Secrets are 64-char hex provisioning_secret from /api/v1/nodes/provision")
		os.Exit(1)
	}

	// Kunci HMAC adalah byte ASCII dari provisioning_secret APA ADANYA, termasuk
	// prefix "sec_": server mengenkripsi []byte(secret) utuh (internal/api:241) dan
	// firmware memakai isi NVS apa adanya sebagai kunci (firmware/src/mqtt.cpp:132).
	// Versi sebelumnya membuang prefix lalu hex-decode, sehingga setiap trigger
	// ditolak "HMAC invalid".
	secrets := [3]string{os.Args[1], os.Args[2], os.Args[3]}

	nodes := [3]struct {
		ID       string
		Lat, Lon float64
	}{
		{"NODE-AAAAAAAA", -6.91, 107.61},
		{"NODE-BBBBBBBB", -6.92, 107.62},
		{"NODE-CCCCCCCC", -6.90, 107.60},
	}

	// Earthquake parameters: PGA=150 gal gives MMI VI (Strong)
	pga := 150.0 // gal
	durMs := int64(3500)
	ts := time.Now().UnixMilli()

	// Connect to Mosquitto broker (dev: plaintext 1883)
	opts := mqtt.NewClientOptions().
		AddBroker("tcp://localhost:1883").
		SetClientID("quakealert-simulator").
		SetCleanSession(true)
	client := mqtt.NewClient(opts)

	if token := client.Connect(); token.Wait() && token.Error() != nil {
		fmt.Fprintf(os.Stderr, "MQTT connect failed: %v\n", token.Error())
		os.Exit(1)
	}
	defer client.Disconnect(250)

	fmt.Printf("Simulating MMI VI earthquake (PGA=%.1f gal) near Bandung...\n", pga)
	fmt.Printf("Timestamp: %d (%s)\n\n", ts, time.UnixMilli(ts).Format(time.RFC3339))

	for i := 0; i < 3; i++ {
		n := nodes[i]
		// Build canonical string: node_id|pga(4dec)|dur_ms|ts
		canonical := fmt.Sprintf("%s|%s|%d|%d", n.ID, strconv.FormatFloat(pga, 'f', 4, 64), durMs, ts)

		// Compute HMAC-SHA256 dengan secret sebagai kunci mentah.
		mac := hmac.New(sha256.New, []byte(secrets[i]))
		mac.Write([]byte(canonical))
		sig := hex.EncodeToString(mac.Sum(nil))

		// Construct trigger payload per contracts/mqtt/trigger.schema.json
		payload := fmt.Sprintf(`{"node_id":"%s","pga":%.4f,"dur_ms":%d,"ts":%d,"signature":"%s"}`, n.ID, pga, durMs, ts, sig)
		topic := fmt.Sprintf("sensor/%s/trigger", n.ID)

		token := client.Publish(topic, 1, false, payload)
		token.Wait()
		if token.Error() != nil {
			fmt.Fprintf(os.Stderr, "Publish failed for %s: %v\n", n.ID, token.Error())
		} else {
			fmt.Printf("✓ Published to %s (PGA=%.1f gal, sig=%s...)\n", topic, pga, sig[:16])
		}
	}

	fmt.Println("\nDone. Check Android Warning tab for EARTHQUAKE_ALERT (WebSocket broadcast)")
	fmt.Println("Check server logs for: 'event didispatch' with CONFIRMED status")
}
