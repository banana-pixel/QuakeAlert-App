package dispatch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// fcmScope adalah OAuth2 scope untuk FCM HTTP v1.
const fcmScope = "https://www.googleapis.com/auth/firebase.messaging"

// FCMSender mengirim data-only message ke FCM HTTP v1. Diabstraksi sebagai
// interface agar dispatch dapat diuji tanpa jaringan.
type FCMSender interface {
	Send(ctx context.Context, msg *FCMMessage) error
}

// FCMMessage merepresentasikan payload sesuai contracts/fcm/alert_payload.json.
// SEMUA field data bertipe string (batasan FCM). android.priority WAJIB HIGH
// untuk life-safety (bypass Doze).
type FCMMessage struct {
	Topic     string            // salah satu dari Topic/Token/Condition
	Token     string            //
	Condition string            //
	Data      map[string]string // data-only
	Priority  string            // "HIGH" | "NORMAL"
}

// BuildAlertData membentuk map data-only sesuai kontrak dari AlertMessage.
// pga_gal 2 desimal, centroid 4 desimal, timestamp ms epoch UTC sebagai string.
func BuildAlertData(a *AlertMessage) map[string]string {
	data := map[string]string{
		"type":            a.Type,
		"event_id":        a.EventID,
		"mmi":             a.MMI,
		"intensity_label": a.IntensityLabel,
		"pga_gal":         strconv.FormatFloat(a.PGAGal, 'f', 2, 64),
		"centroid_lat":    strconv.FormatFloat(a.CentroidLat, 'f', 4, 64),
		"centroid_lon":    strconv.FormatFloat(a.CentroidLon, 'f', 4, 64),
		"location_name":   a.LocationName,
		"timestamp":       strconv.FormatInt(a.Timestamp, 10),
	}
	// Hanya ditambahkan bila benar: payload gempa sungguhan tetap identik
	// dengan kontrak yang sudah dipasang klien terdahulu, dan tidak ada
	// "is_test": "false" yang bisa salah dibaca sebagai ada-nilainya.
	if a.IsTest {
		data["is_test"] = "true"
	}
	// Field siklus hidup Fase 3 (§8.3), dengan aturan yang sama seperti is_test:
	// hanya ditambahkan bila BERISI. Payload jalur Fase 2 karenanya tetap
	// identik dengan kontrak yang sudah dipasang klien terdahulu, dan tidak ada
	// "event_state": "" yang dapat dibaca sebagai state bernama.
	if a.EventState != "" {
		data["event_state"] = a.EventState
	}
	if a.EventRevision > 0 {
		data["event_revision"] = strconv.Itoa(a.EventRevision)
	}
	if a.OriginTS > 0 {
		data["origin_ts"] = strconv.FormatInt(a.OriginTS, 10)
	}
	if a.OriginTSSource != "" {
		data["origin_ts_source"] = a.OriginTSSource
	}
	if a.IndependentCellCount > 0 {
		data["independent_cell_count"] = strconv.Itoa(a.IndependentCellCount)
	}
	return data
}

// HTTPV1Sender adalah implementasi FCMSender via FCM HTTP v1 (Admin) memakai
// service account (OAuth2). Ringan: hanya golang.org/x/oauth2 + net/http.
type HTTPV1Sender struct {
	httpClient *http.Client
	endpoint   string
	log        *slog.Logger
}

// NewHTTPV1Sender membuat sender dari kredensial service account (JSON).
// httpClient di-inject dengan oauth2.Transport sehingga setiap request otomatis
// membawa Bearer token yang di-refresh.
func NewHTTPV1Sender(ctx context.Context, projectID string, serviceAccountJSON []byte, log *slog.Logger) (*HTTPV1Sender, error) {
	creds, err := google.CredentialsFromJSON(ctx, serviceAccountJSON, fcmScope)
	if err != nil {
		return nil, fmt.Errorf("parse service account: %w", err)
	}
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &oauth2.Transport{
			Source: creds.TokenSource,
			Base:   http.DefaultTransport,
		},
	}
	return &HTTPV1Sender{
		httpClient: client,
		endpoint:   fmt.Sprintf("https://fcm.googleapis.com/v1/projects/%s/messages:send", projectID),
		log:        log,
	}, nil
}

// Send mengirim pesan. Menyusun envelope JSON FCM v1 dari FCMMessage.
func (s *HTTPV1Sender) Send(ctx context.Context, msg *FCMMessage) error {
	inner := map[string]any{
		"data": msg.Data,
		"android": map[string]any{
			"priority": msg.Priority,
		},
	}
	switch {
	case msg.Topic != "":
		inner["topic"] = msg.Topic
	case msg.Token != "":
		inner["token"] = msg.Token
	case msg.Condition != "":
		inner["condition"] = msg.Condition
	default:
		return fmt.Errorf("fcm: salah satu topic/token/condition wajib diisi")
	}
	body, err := json.Marshal(map[string]any{"message": inner})
	if err != nil {
		return fmt.Errorf("marshal fcm message: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("kirim fcm: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("fcm status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}
