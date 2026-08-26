package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/banana-pixel/quakealert/server/internal/store"
)

// fakeFanout mencatat siaran agar urutan "simpan lalu siarkan" bisa diuji.
type fakeFanout struct{ events []ChatEvent }

func (f *fakeFanout) BroadcastChat(e ChatEvent) { f.events = append(f.events, e) }

// chatServer membangun Server chat lengkap dengan router terautentikasi.
func chatServer(repo Repo, limiter RateLimiter, fanout ChatFanout) (*Server, http.Handler) {
	srv := NewServer(repo, fakeDecryptCipher{}, limiter,
		MQTTPublic{Broker: "b", Port: 8883, TLS: true},
		AuthConfig{JWTSecret: []byte(testSecret), TokenTTL: testTokenTTL},
		testLogger())
	if fanout != nil {
		srv.SetChatFanout(fanout)
	}
	h := srv.Router(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusSwitchingProtocols)
	}, testLogger())
	return srv, h
}

func TestRegionCode_NormalisesSpellingVariants(t *testing.T) {
	// Dua ejaan provinsi yang sama harus mendarat di satu kanal, bukan dua ruang
	// yang masing-masing terasa kosong.
	for _, in := range []string{"Jawa Barat", "jawa  barat", "  JAWA-BARAT ", "Jawa/Barat"} {
		if got := RegionCode("id", in); got != "ID-jawa-barat" {
			t.Fatalf("RegionCode(%q) = %q, mau ID-jawa-barat", in, got)
		}
	}
}

func TestRegionCode_RejectsWhatItCannotName(t *testing.T) {
	cases := map[string][2]string{
		"negara bukan ISO2": {"Indonesia", "Jawa Barat"},
		"negara kosong":     {"", "Jawa Barat"},
		"wilayah kosong":    {"ID", "   "},
		"wilayah non-ASCII": {"JP", "東京都"},
	}
	for name, c := range cases {
		if got := RegionCode(c[0], c[1]); got != "" {
			t.Fatalf("%s: RegionCode(%q,%q) = %q, mau kosong", name, c[0], c[1], got)
		}
	}
}

func TestRegionCode_FitsTheChannelIDColumn(t *testing.T) {
	long := "Daerah Istimewa Yang Namanya Sengaja Dibuat Sangat Panjang Sekali Melebihi Kolom"
	got := RegionCode("ID", long)
	if len(got) > maxRegionCodeLen {
		t.Fatalf("region code %d karakter (%q), maksimal %d", len(got), got, maxRegionCodeLen)
	}
	if got[len(got)-1] == '-' {
		t.Fatalf("region code berakhir dengan tanda hubung: %q", got)
	}
}

func TestUpdateLocation_DerivesTheRegionalChannel(t *testing.T) {
	repo := &fakeRepo{locUpdated: time.Unix(1_781_913_558, 0).UTC()}
	h := newTestServer(repo, NewMemoryRateLimiter())

	body := `{"latitude":-6.9175,"longitude":107.6191,"location_name":"Bandung",` +
		`"country_iso":"ID","admin_area":"Jawa Barat"}`
	rec := do(h, authedRequest(http.MethodPut, "/api/v1/users/location", body, testSecret, "u-1"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200: %s", rec.Code, rec.Body.String())
	}
	if repo.regionCode != "ID-jawa-barat" {
		t.Fatalf("region tersimpan = %q, mau ID-jawa-barat", repo.regionCode)
	}
	var resp updateLocationResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.RegionCode == nil || *resp.RegionCode != "ID-jawa-barat" {
		t.Fatalf("region_code respons = %v, mau ID-jawa-barat", resp.RegionCode)
	}
}

func TestUpdateLocation_RegistersTheRegionalChannelName(t *testing.T) {
	// Tanpa langkah ini baris katalog tidak pernah ada, dan header chat berbunyi
	// "ID-jawa-barat" alih-alih "Jawa Barat": kunci kanal sudah di-slug dan tidak
	// bisa dibalik, jadi inilah satu-satunya momen nama itu ada di server.
	repo := &fakeRepo{locUpdated: time.Unix(1_781_913_558, 0).UTC()}
	h := newTestServer(repo, NewMemoryRateLimiter())

	body := `{"latitude":-6.9175,"longitude":107.6191,` +
		`"country_iso":"ID","admin_area":"  Jawa   Barat "}`
	rec := do(h, authedRequest(http.MethodPut, "/api/v1/users/location", body, testSecret, "u-1"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200: %s", rec.Code, rec.Body.String())
	}
	if repo.ensuredChan != "ID-jawa-barat" || repo.ensuredKind != store.ChannelKindRegional {
		t.Fatalf("kanal terdaftar = %q/%q, mau ID-jawa-barat/REGIONAL", repo.ensuredChan, repo.ensuredKind)
	}
	if repo.ensuredName != "Jawa Barat" {
		t.Fatalf("nama tampilan = %q, mau %q", repo.ensuredName, "Jawa Barat")
	}
}

func TestUpdateLocation_UnnameableRegionRegistersNothing(t *testing.T) {
	// Slug kosong berarti tidak ada kanal regional sama sekali; mendaftarkan baris
	// permanen berjudul kunci kanal akan menang atas ejaan benar dari klien
	// berikutnya (penulis pertama yang menang).
	repo := &fakeRepo{locUpdated: time.Unix(1_781_913_558, 0).UTC()}
	h := newTestServer(repo, NewMemoryRateLimiter())

	body := `{"latitude":-6.9175,"longitude":107.6191,"country_iso":"ID","admin_area":"日本語"}`
	rec := do(h, authedRequest(http.MethodPut, "/api/v1/users/location", body, testSecret, "u-1"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200", rec.Code)
	}
	if repo.ensureCalls != 0 {
		t.Fatalf("EnsureChatChannel dipanggil %d kali, mau 0", repo.ensureCalls)
	}
}

func TestRegionDisplayName_KeepsTheGeocoderSpelling(t *testing.T) {
	// Title-case akan merusak "DKI Jakarta"; geocoder sudah mengirim ejaan resmi.
	cases := map[string]string{
		"  Jawa   Barat ": "Jawa Barat",
		"DKI Jakarta":     "DKI Jakarta",
		"   ":             "",
	}
	for in, want := range cases {
		if got := RegionDisplayName(in); got != want {
			t.Fatalf("RegionDisplayName(%q) = %q, mau %q", in, got, want)
		}
	}
	long := RegionDisplayName(strings.Repeat("a", maxDisplayNameLen+20))
	if len(long) > maxDisplayNameLen {
		t.Fatalf("nama tampilan %d karakter, maksimal %d", len(long), maxDisplayNameLen)
	}
}

func TestUpdateLocation_RegionFailureDoesNotFailTheSync(t *testing.T) {
	// Lokasi menopang penargetan alert; keanggotaan kanal chat tidak boleh
	// menjadi alasan sebuah sinkronisasi lokasi dilaporkan gagal.
	repo := &fakeRepo{
		locUpdated:   time.Unix(1_781_913_558, 0).UTC(),
		regionSetErr: errors.New("boom"),
	}
	h := newTestServer(repo, NewMemoryRateLimiter())

	body := `{"latitude":-6.9175,"longitude":107.6191,"country_iso":"ID","admin_area":"Jawa Barat"}`
	rec := do(h, authedRequest(http.MethodPut, "/api/v1/users/location", body, testSecret, "u-1"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200", rec.Code)
	}
	var resp updateLocationResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.RegionCode != nil {
		t.Fatalf("region_code = %v, mau null karena tidak tersimpan", *resp.RegionCode)
	}
}

func TestListChatChannels_AnswersWhatTheClientMayNotGuess(t *testing.T) {
	repo := &fakeRepo{chatChannels: []store.ChatChannel{
		{ChannelID: "global", Kind: store.ChannelKindGlobal, DisplayName: "Global"},
		{ChannelID: "ID-jawa-barat", Kind: store.ChannelKindRegional, DisplayName: "Jawa Barat"},
	}}
	_, h := chatServer(repo, NewMemoryRateLimiter(), nil)

	rec := do(h, authedRequest(http.MethodGet, "/api/v1/chat/channels", "", testSecret, "u-1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200: %s", rec.Code, rec.Body.String())
	}
	var resp listChatChannelsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Channels) != 2 || resp.Channels[0].ChannelID != "global" {
		t.Fatalf("kanal = %+v, mau global lebih dulu lalu regional", resp.Channels)
	}
}

func TestChatMessages_GlobalIsReadableWithoutAFix(t *testing.T) {
	// Kanal global adalah satu-satunya yang berfungsi sebelum ada fix lokasi.
	repo := &fakeRepo{chatIdentity: &store.UserChatIdentity{Pseudonym: "AnonimTenang"}}
	_, h := chatServer(repo, NewMemoryRateLimiter(), nil)

	rec := do(h, authedRequest(http.MethodGet, "/api/v1/chat/messages", "", testSecret, "u-1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200: %s", rec.Code, rec.Body.String())
	}
	if repo.chatMsgChan != store.GlobalChannelID {
		t.Fatalf("kanal default = %q, mau global", repo.chatMsgChan)
	}
	if repo.chatMsgLimit != store.DefaultChatLimit {
		t.Fatalf("limit default = %d, mau %d", repo.chatMsgLimit, store.DefaultChatLimit)
	}
}

func TestChatMessages_ForeignRegionalChannelIsForbidden(t *testing.T) {
	repo := &fakeRepo{chatIdentity: &store.UserChatIdentity{
		Pseudonym:  "AnonimTenang",
		RegionCode: "ID-jawa-barat",
	}}
	_, h := chatServer(repo, NewMemoryRateLimiter(), nil)

	rec := do(h, authedRequest(
		http.MethodGet, "/api/v1/chat/messages?channel_id=ID-bali", "", testSecret, "u-1"))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, mau 403: %s", rec.Code, rec.Body.String())
	}
}

func TestChatMessages_LimitIsCappedAndCursorIsParsed(t *testing.T) {
	repo := &fakeRepo{}
	_, h := chatServer(repo, NewMemoryRateLimiter(), nil)

	rec := do(h, authedRequest(http.MethodGet,
		"/api/v1/chat/messages?limit=9999&before=2026-06-20T00:19:18Z", "", testSecret, "u-1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200: %s", rec.Code, rec.Body.String())
	}
	if repo.chatMsgLimit != store.MaxChatLimit {
		t.Fatalf("limit = %d, mau dibatasi ke %d", repo.chatMsgLimit, store.MaxChatLimit)
	}
	if repo.chatMsgBefore == nil || !repo.chatMsgBefore.Equal(time.Date(2026, 6, 20, 0, 19, 18, 0, time.UTC)) {
		t.Fatalf("kursor before = %v", repo.chatMsgBefore)
	}

	bad := do(h, authedRequest(http.MethodGet,
		"/api/v1/chat/messages?before=20-06-2026", "", testSecret, "u-1"))
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("kursor cacat: status = %d, mau 400", bad.Code)
	}
}

func TestCreateChatMessage_PersistsThenBroadcasts(t *testing.T) {
	repo := &fakeRepo{chatIdentity: &store.UserChatIdentity{
		Pseudonym:    "AnonimTenang",
		RegionCode:   "ID-jawa-barat",
		LocationName: "Bandung",
	}}
	fanout := &fakeFanout{}
	_, h := chatServer(repo, NewMemoryRateLimiter(), fanout)

	body := `{"channel_id":"ID-jawa-barat","message":"  aman di sini  ","client_message_id":"c-1"}`
	rec := do(h, authedRequest(http.MethodPost, "/api/v1/chat/messages", body, testSecret, "u-1"))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, mau 201: %s", rec.Code, rec.Body.String())
	}
	if repo.insertedBody != "aman di sini" {
		t.Fatalf("isi tersimpan = %q, mau tanpa spasi tepi", repo.insertedBody)
	}
	if repo.insertedClID != "c-1" {
		t.Fatalf("client_message_id = %q, mau diteruskan untuk idempotensi", repo.insertedClID)
	}
	if len(fanout.events) != 1 || fanout.events[0].ChannelID != "ID-jawa-barat" {
		t.Fatalf("siaran = %+v, mau satu frame di kanal yang sama", fanout.events)
	}
}

func TestCreateChatMessage_RejectsWhatItCannotStore(t *testing.T) {
	cases := map[string]string{
		"kosong":        `{"message":"   "}`,
		"terlalu besar": `{"message":"` + longRunes(maxChatBodyLen+1) + `"}`,
	}
	for name, body := range cases {
		repo := &fakeRepo{}
		_, h := chatServer(repo, NewMemoryRateLimiter(), nil)
		rec := do(h, authedRequest(http.MethodPost, "/api/v1/chat/messages", body, testSecret, "u-1"))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: status = %d, mau 400", name, rec.Code)
		}
		if repo.insertedBody != "" {
			t.Fatalf("%s: pesan cacat tetap tersimpan", name)
		}
	}
}

func TestCreateChatMessage_ForeignChannelIsForbiddenBeforeTheWrite(t *testing.T) {
	repo := &fakeRepo{chatIdentity: &store.UserChatIdentity{Pseudonym: "AnonimTenang"}}
	_, h := chatServer(repo, NewMemoryRateLimiter(), nil)

	body := `{"channel_id":"ID-bali","message":"halo"}`
	rec := do(h, authedRequest(http.MethodPost, "/api/v1/chat/messages", body, testSecret, "u-1"))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, mau 403: %s", rec.Code, rec.Body.String())
	}
	if repo.insertedBody != "" {
		t.Fatalf("pesan ke kanal asing tetap tersimpan")
	}
}

func TestCreateChatMessage_SecondSendWithinTheWindowIsThrottled(t *testing.T) {
	repo := &fakeRepo{}
	_, h := chatServer(repo, NewMemoryRateLimiter(), nil)

	body := `{"message":"halo"}`
	first := do(h, authedRequest(http.MethodPost, "/api/v1/chat/messages", body, testSecret, "u-1"))
	if first.Code != http.StatusCreated {
		t.Fatalf("kiriman pertama: status = %d, mau 201", first.Code)
	}
	second := do(h, authedRequest(http.MethodPost, "/api/v1/chat/messages", body, testSecret, "u-1"))
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("kiriman kedua: status = %d, mau 429", second.Code)
	}
	// Rem bersifat per-user: user lain tidak boleh ikut terkena.
	other := do(h, authedRequest(http.MethodPost, "/api/v1/chat/messages", body, testSecret, "u-2"))
	if other.Code != http.StatusCreated {
		t.Fatalf("user lain: status = %d, mau 201", other.Code)
	}
}

func TestCreateChatMessage_MalformedRequestDoesNotSpendTheSendQuota(t *testing.T) {
	repo := &fakeRepo{}
	_, h := chatServer(repo, NewMemoryRateLimiter(), nil)

	bad := do(h, authedRequest(http.MethodPost, "/api/v1/chat/messages",
		`{"message":""}`, testSecret, "u-1"))
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, mau 400", bad.Code)
	}
	good := do(h, authedRequest(http.MethodPost, "/api/v1/chat/messages",
		`{"message":"halo"}`, testSecret, "u-1"))
	if good.Code != http.StatusCreated {
		t.Fatalf("kiriman valid setelahnya: status = %d, mau 201", good.Code)
	}
}

func TestChatRoutesRequireAuth(t *testing.T) {
	_, h := chatServer(&fakeRepo{}, NewMemoryRateLimiter(), nil)
	for _, target := range []string{"/api/v1/chat/channels", "/api/v1/chat/messages"} {
		rec := do(h, expiredRequest(http.MethodGet, target, "", testSecret, "u-1"))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s: status = %d, mau 401", target, rec.Code)
		}
	}
}

// longRunes membuat string sepanjang n rune multi-byte, sehingga batas panjang
// benar-benar teruji dalam rune: 501 rune ini adalah ~1002 byte, jadi pemeriksaan
// berbasis byte akan lolos di sini padahal seharusnya menolak.
func longRunes(n int) string {
	out := make([]rune, n)
	for i := range out {
		out[i] = 'é'
	}
	return string(out)
}

func TestUpdateLocation_KeepsTheRegionWhenNoPlaceIsSent(t *testing.T) {
	// Reverse-geocode yang gagal di ponsel adalah kondisi sesaat, bukan bukti
	// bahwa user pindah provinsi. Sinkronisasi tanpa wilayah karena itu tidak
	// boleh mengeluarkannya dari ruang chat wilayahnya.
	//
	// locMovedKm sengaja nil: profil belum punya posisi sebelumnya, jadi jarak
	// pindahnya tidak diketahui — dan yang tidak diketahui tidak boleh
	// membatalkan keanggotaan. Kasus "pindah sedikit" ada di
	// TestUpdateLocation_SmallMoveKeepsTheRegion.
	repo := &fakeRepo{
		locUpdated:   time.Unix(1_781_913_558, 0).UTC(),
		chatIdentity: &store.UserChatIdentity{Pseudonym: "AnonimTenang", RegionCode: "ID-jawa-barat"},
	}
	h := newTestServer(repo, NewMemoryRateLimiter())

	body := `{"latitude":-6.9175,"longitude":107.6191,"location_name":"Bandung"}`
	rec := do(h, authedRequest(http.MethodPut, "/api/v1/users/location", body, testSecret, "u-1"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200: %s", rec.Code, rec.Body.String())
	}
	if repo.regionSets != 0 {
		t.Fatalf("SetUserRegion dipanggil %d kali, mau 0", repo.regionSets)
	}
	var resp updateLocationResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.RegionCode == nil || *resp.RegionCode != "ID-jawa-barat" {
		t.Fatalf("region_code respons = %v, mau kanal yang tersimpan", resp.RegionCode)
	}
}

func TestUpdateLocation_SmallMoveKeepsTheRegion(t *testing.T) {
	// Perjalanan harian di dalam provinsi yang sama, tanpa wilayah karena geocoder
	// gagal: keanggotaan kanal tidak boleh ikut hilang.
	moved := 8.0
	repo := &fakeRepo{
		locUpdated:   time.Unix(1_781_913_558, 0).UTC(),
		locMovedKm:   &moved,
		chatIdentity: &store.UserChatIdentity{Pseudonym: "AnonimTenang", RegionCode: "ID-jawa-barat"},
	}
	h := newTestServer(repo, NewMemoryRateLimiter())

	body := `{"latitude":-6.9175,"longitude":107.6191}`
	rec := do(h, authedRequest(http.MethodPut, "/api/v1/users/location", body, testSecret, "u-1"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200: %s", rec.Code, rec.Body.String())
	}
	if repo.regionSets != 0 {
		t.Fatalf("SetUserRegion dipanggil %d kali, mau 0", repo.regionSets)
	}
	var resp updateLocationResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.RegionCode == nil || *resp.RegionCode != "ID-jawa-barat" {
		t.Fatalf("region_code respons = %v, mau kanal yang tersimpan", resp.RegionCode)
	}
}

// Inti bug yang diperbaiki: region_code dulunya write-once. Sekali sebuah profil
// mendapat region dari satu geocode yang berhasil, tidak ada jalur lain yang bisa
// menimpanya — jadi user yang pindah provinsi sementara geocoder-nya gagal tetap
// membaca dan menulis di ruang chat provinsi yang sudah ia tinggalkan, selamanya.
func TestUpdateLocation_BigMoveWithoutAPlaceClearsTheRegion(t *testing.T) {
	moved := regionInvalidateKm + 1
	repo := &fakeRepo{
		locUpdated:   time.Unix(1_781_913_558, 0).UTC(),
		locMovedKm:   &moved,
		chatIdentity: &store.UserChatIdentity{Pseudonym: "AnonimTenang", RegionCode: "ID-jawa-tengah"},
	}
	h := newTestServer(repo, NewMemoryRateLimiter())

	body := `{"latitude":-6.9175,"longitude":107.6191}`
	rec := do(h, authedRequest(http.MethodPut, "/api/v1/users/location", body, testSecret, "u-1"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200: %s", rec.Code, rec.Body.String())
	}
	if repo.regionSets != 1 || repo.regionCode != "" {
		t.Fatalf("region = %q setelah %d penulisan, mau dikosongkan sekali",
			repo.regionCode, repo.regionSets)
	}
	var resp updateLocationResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.RegionCode != nil {
		t.Fatalf("region_code respons = %v, mau null", *resp.RegionCode)
	}
}

// Perpindahan jauh pada profil yang belum punya region tidak perlu penulisan apa
// pun: tidak ada keanggotaan basi untuk dibatalkan.
func TestUpdateLocation_BigMoveWithNoStoredRegionWritesNothing(t *testing.T) {
	moved := regionInvalidateKm + 500
	repo := &fakeRepo{
		locUpdated:   time.Unix(1_781_913_558, 0).UTC(),
		locMovedKm:   &moved,
		chatIdentity: &store.UserChatIdentity{Pseudonym: "AnonimTenang"},
	}
	h := newTestServer(repo, NewMemoryRateLimiter())

	body := `{"latitude":-6.9175,"longitude":107.6191}`
	rec := do(h, authedRequest(http.MethodPut, "/api/v1/users/location", body, testSecret, "u-1"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200: %s", rec.Code, rec.Body.String())
	}
	if repo.regionSets != 0 {
		t.Fatalf("SetUserRegion dipanggil %d kali, mau 0", repo.regionSets)
	}
}

// Wilayah yang dikirim selalu menurunkan ulang region, sejauh apa pun pindahnya:
// itu satu-satunya bukti langsung tentang provinsi user yang pernah dimiliki
// server.
func TestUpdateLocation_APlaceAlwaysRederivesTheRegion(t *testing.T) {
	moved := regionInvalidateKm + 100
	repo := &fakeRepo{
		locUpdated:   time.Unix(1_781_913_558, 0).UTC(),
		locMovedKm:   &moved,
		chatIdentity: &store.UserChatIdentity{Pseudonym: "AnonimTenang", RegionCode: "ID-jawa-tengah"},
	}
	h := newTestServer(repo, NewMemoryRateLimiter())

	body := `{"latitude":-6.9175,"longitude":107.6191,"country_iso":"ID","admin_area":"Jawa Barat"}`
	rec := do(h, authedRequest(http.MethodPut, "/api/v1/users/location", body, testSecret, "u-1"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200: %s", rec.Code, rec.Body.String())
	}
	if repo.regionCode != "ID-jawa-barat" {
		t.Fatalf("region tersimpan = %q, mau ID-jawa-barat", repo.regionCode)
	}
}

func TestUpdateLocation_UnnameablePlaceClearsTheRegion(t *testing.T) {
	// Kasus sebaliknya: geocoder menjawab, tetapi dengan sesuatu yang tidak bisa
	// dinamai menjadi kunci kanal. Region memang dikosongkan — kanal regional
	// yang salah lebih buruk daripada tidak ada kanal regional.
	repo := &fakeRepo{
		locUpdated:   time.Unix(1_781_913_558, 0).UTC(),
		regionCode:   "ID-jawa-barat",
		chatIdentity: &store.UserChatIdentity{Pseudonym: "AnonimTenang", RegionCode: "ID-jawa-barat"},
	}
	h := newTestServer(repo, NewMemoryRateLimiter())

	body := `{"latitude":-6.9175,"longitude":107.6191,"country_iso":"ID","admin_area":"日本語"}`
	rec := do(h, authedRequest(http.MethodPut, "/api/v1/users/location", body, testSecret, "u-1"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200", rec.Code)
	}
	if repo.regionSets != 1 || repo.regionCode != "" {
		t.Fatalf("region tersimpan = %q setelah %d penulisan, mau kosong sekali", repo.regionCode, repo.regionSets)
	}
	var resp updateLocationResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.RegionCode != nil {
		t.Fatalf("region_code = %v, mau null", *resp.RegionCode)
	}
}
