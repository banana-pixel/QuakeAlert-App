//go:build ignore

// Helper manual: dijalankan lewat `go run scripts/simulate_heartbeat.go`.
// Tag ignore dipasang karena file ini dan setup_nodes.go sama-sama `package main`
// dalam satu direktori.
//
// Menerbitkan heartbeat periodik untuk node uji sehingga /sensors menunjukkan
// mereka Online dan badge kesehatan di aplikasi naik ke Healthy. Heartbeat tidak
// terautentikasi sesuai kontrak (contracts/mqtt/heartbeat.schema.json) — ia hanya
// boleh memutakhirkan telemetri liveness, bukan dipakai untuk keputusan
// life-safety; simulator ini memakai jalur yang sama persis dengan firmware.
//
// Pemakaian:
//
//	go run scripts/simulate_heartbeat.go                    # tiga node uji bawaan
//	go run scripts/simulate_heartbeat.go NODE-163A149F ...  # id eksplisit
package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

var defaultNodes = []string{"NODE-AAAAAAAA", "NODE-BBBBBBBB", "NODE-CCCCCCCC"}

func main() {
	nodes := os.Args[1:]
	if len(nodes) == 0 {
		nodes = defaultNodes
	}

	client := mqtt.NewClient(mqtt.NewClientOptions().
		AddBroker("tcp://localhost:1883").
		SetClientID("quakealert-heartbeat-simulator").
		SetCleanSession(true))
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		fmt.Fprintf(os.Stderr, "MQTT connect failed: %v\n", token.Error())
		os.Exit(1)
	}
	defer client.Disconnect(250)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	start := time.Now()
	publish := func() {
		ts := time.Now().UnixMilli()
		for i, id := range nodes {
			payload := fmt.Sprintf(
				`{"id":%q,"rssi":-60,"uptime_s":%d,"ts":%d}`,
				id, int64(time.Since(start).Seconds())+uintJitter(i), ts)
			token := client.Publish("sensor/"+id+"/heartbeat", 1, false, payload)
			token.Wait()
		}
		fmt.Fprintf(os.Stderr, "%s heartbeat terkirim untuk %d node\n",
			time.Now().Format("15:04:05"), len(nodes))
	}

	publish()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	fmt.Fprintf(os.Stderr, "simulator berjalan — Ctrl-C untuk berhenti\n")
	for {
		select {
		case <-ticker.C:
			publish()
		case <-stop:
			return
		}
	}
}

// uintJitter memberi uptime sedikit berbeda per node agar baris /sensors tidak
// identik byte-per-byte (memudahkan membaca log saat debugging).
func uintJitter(i int) int64 { return int64(3600 + i*97) }
