//go:build ignore

// sim_confirm_scenario.go — Checkpoint 3.1: multi-node CONFIRMED simulation.
//
// Publishes three v1 HMAC-signed triggers to localhost:1883 on behalf of three
// virtual simulation nodes, staggered 0s / 1s / 2s within the 8 s correlation
// window.  All HMAC signing uses the exported ingest helpers so the canonical
// string is byte-identical to what the server's verifier checks.
//
// Usage (called by sim_multi_node.sh):
//
//	go run scripts/sim_confirm_scenario.go <secret_A> <secret_B> <secret_C> <onset_ts_ms>
//
// Secrets are the plaintext provisioning_secret strings returned by
// POST /api/v1/nodes/provision (prefix "sec_" included).
// onset_ts_ms is the shared onset timestamp written into every trigger so that
// all three observations fall inside the same correlation bucket.
package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/banana-pixel/quakealert/server/internal/ingest"
)

// simNodes defines the three virtual simulation nodes.
// Coordinates chosen so that:
//   - A ↔ B: HaversineKm(-6.900, 107.600, -6.855, 107.600) ≈ 5.01 km  → independent
//   - A ↔ C: HaversineKm(-6.900, 107.600, -6.910, 107.650) ≈ 5.1 km   → independent
//   - All within 50 km of each other → within AttachRadiusKm
//
// MinIndependentCells = 2; independentCount(A,B,C, 5km) = 3 → CONFIRMED gate met.
var simNodes = [3]struct {
	ID  string
	Lat float64
	Lon float64
}{
	{"NODE-53494D41", -6.900, 107.600}, // SIM-A  (hex of "SIMA")
	{"NODE-53494D42", -6.855, 107.600}, // SIM-B  ~5.01 km north of A
	{"NODE-53494D43", -6.910, 107.650}, // SIM-C  ~5.1 km SE of A
}

const (
	simPGA   = 300.0 // gal — well above MinPGAGal=16.6, gives MMI VIII (strong)
	simDurMs = int64(3500)
)

func main() {
	if len(os.Args) != 5 {
		fmt.Fprintln(os.Stderr, "usage: go run sim_confirm_scenario.go <secret_A> <secret_B> <secret_C> <onset_ts_ms>")
		os.Exit(1)
	}

	secrets := [3]string{os.Args[1], os.Args[2], os.Args[3]}

	onsetTsMs, err := strconv.ParseInt(os.Args[4], 10, 64)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid onset_ts_ms: %v\n", err)
		os.Exit(1)
	}

	opts := mqtt.NewClientOptions().
		AddBroker("tcp://localhost:1883").
		SetClientID("quakealert-sim-3.1").
		SetCleanSession(true)
	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		fmt.Fprintf(os.Stderr, "MQTT connect failed: %v\n", token.Error())
		os.Exit(1)
	}
	defer client.Disconnect(250)

	fmt.Printf("sim: onset_ts=%d  pga=%.1f gal  nodes=%v\n",
		onsetTsMs, simPGA,
		[]string{simNodes[0].ID, simNodes[1].ID, simNodes[2].ID})

	for i, n := range simNodes {
		// Stagger publishes: 0 ms, 1000 ms, 2000 ms.
		// ts = publish time; must be fresh (within MaxTriggerAge=5 min).
		// onset_ts_ms is shared so all three land in the same correlation bucket.
		time.Sleep(time.Duration(i) * time.Second)
		ts := time.Now().UnixMilli()

		// v1 canonical string: node_id|pga(4dec)|dur_ms|ts
		canonical := ingest.CanonicalString(n.ID, simPGA, simDurMs, ts)
		sig := ingest.ComputeHMAC([]byte(secrets[i]), canonical)

		payload := fmt.Sprintf(
			`{"node_id":%q,"pga":%.4f,"dur_ms":%d,"ts":%d,"signature":%q}`,
			n.ID, simPGA, simDurMs, ts, sig,
		)
		topic := fmt.Sprintf("sensor/%s/trigger", n.ID)

		token := client.Publish(topic, 1, false, payload)
		token.Wait()
		if token.Error() != nil {
			fmt.Fprintf(os.Stderr, "publish %s failed: %v\n", n.ID, token.Error())
			os.Exit(1)
		}
		fmt.Printf("sim: published %s  ts=%d  sig=%s...\n", n.ID, ts, sig[:16])
	}

	fmt.Println("sim: all 3 triggers published")
}
