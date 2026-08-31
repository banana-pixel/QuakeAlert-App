//go:build ignore

// sim_dual_event_scenario.go — Checkpoint 3.2: two simultaneous earthquake events.
//
// Publishes triggers for TWO geographically separate clusters concurrently.
// Cluster A (Bandung, ~-6.9°/107.6°) and Cluster B (Surabaya, ~-7.25°/112.75°)
// are ≈570 km apart — well beyond AttachRadiusKm=50 — so each cluster forms
// its own event_id.
//
// Usage:
//
//	go run scripts/sim_dual_event_scenario.go \
//	    <secA1> <secA2> <secA3> \
//	    <secB1> <secB2> <secB3>
//
// Both clusters publish within the same 8 s correlation window.
// Cluster A publishes at t=0/1/2s, Cluster B at t=0.5/1.5/2.5s (interleaved)
// to demonstrate concurrent processing.
package main

import (
	"fmt"
	"os"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/banana-pixel/quakealert/server/internal/ingest"
)

// Cluster A — Bandung (same IDs as 3.1)
var clusterA = [3]struct {
	ID  string
	Lat float64
	Lon float64
}{
	{"NODE-53494D41", -6.900, 107.600},
	{"NODE-53494D42", -6.855, 107.600},
	{"NODE-53494D43", -6.910, 107.650},
}

// Cluster B — Surabaya
// B1↔B2: HaversineKm(-7.250,112.750, -7.205,112.750) ≈ 5.01 km → independent
// B1↔B3: HaversineKm(-7.250,112.750, -7.260,112.800) ≈ 5.5 km  → independent
var clusterB = [3]struct {
	ID  string
	Lat float64
	Lon float64
}{
	{"NODE-53554241", -7.250, 112.750},
	{"NODE-53554242", -7.205, 112.750},
	{"NODE-53554243", -7.260, 112.800},
}

const (
	dualPGA   = 300.0
	dualDurMs = int64(3500)
)

func publishCluster(
	client mqtt.Client,
	nodes [3]struct {
		ID  string
		Lat float64
		Lon float64
	},
	secrets [3]string,
	delayOffset time.Duration,
	label string,
) {
	for i, n := range nodes {
		time.Sleep(time.Duration(i)*time.Second + delayOffset)
		ts := time.Now().UnixMilli()
		canonical := ingest.CanonicalString(n.ID, dualPGA, dualDurMs, ts)
		sig := ingest.ComputeHMAC([]byte(secrets[i]), canonical)
		payload := fmt.Sprintf(
			`{"node_id":%q,"pga":%.4f,"dur_ms":%d,"ts":%d,"signature":%q}`,
			n.ID, dualPGA, dualDurMs, ts, sig,
		)
		topic := fmt.Sprintf("sensor/%s/trigger", n.ID)
		token := client.Publish(topic, 1, false, payload)
		token.Wait()
		if token.Error() != nil {
			fmt.Fprintf(os.Stderr, "%s publish %s failed: %v\n", label, n.ID, token.Error())
			os.Exit(1)
		}
		fmt.Printf("sim: [%s] published %s  ts=%d  sig=%s...\n", label, n.ID, ts, sig[:16])
	}
}

func main() {
	if len(os.Args) != 7 {
		fmt.Fprintln(os.Stderr,
			"usage: go run sim_dual_event_scenario.go "+
				"<secA1> <secA2> <secA3> <secB1> <secB2> <secB3>")
		os.Exit(1)
	}

	secretsA := [3]string{os.Args[1], os.Args[2], os.Args[3]}
	secretsB := [3]string{os.Args[4], os.Args[5], os.Args[6]}

	opts := mqtt.NewClientOptions().
		AddBroker("tcp://localhost:1883").
		SetClientID("quakealert-sim-3.2").
		SetCleanSession(true)
	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		fmt.Fprintf(os.Stderr, "MQTT connect failed: %v\n", token.Error())
		os.Exit(1)
	}
	defer client.Disconnect(250)

	fmt.Printf("sim 3.2: cluster-A=%v cluster-B=%v pga=%.1f\n",
		[3]string{clusterA[0].ID, clusterA[1].ID, clusterA[2].ID},
		[3]string{clusterB[0].ID, clusterB[1].ID, clusterB[2].ID},
		dualPGA)

	// Publish cluster A triggers at t=0,1,2s and cluster B at t=0,1,2s concurrently.
	// Run both in goroutines starting simultaneously.
	done := make(chan struct{}, 2)

	go func() {
		publishCluster(client, clusterA, secretsA, 0, "EVENT-A")
		done <- struct{}{}
	}()

	// Slight offset so A's first trigger arrives before B's — ensures UNCONFIRMED
	// for A is observed before B starts. Not required for correctness but makes
	// log inspection cleaner.
	time.Sleep(500 * time.Millisecond)

	go func() {
		publishCluster(client, clusterB, secretsB, 0, "EVENT-B")
		done <- struct{}{}
	}()

	<-done
	<-done
	fmt.Println("sim 3.2: all 6 triggers published")
}
