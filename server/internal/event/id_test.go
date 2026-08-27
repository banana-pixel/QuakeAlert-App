package event

import (
	"regexp"
	"testing"
)

var uuidV4Re = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestNewEventIDIsUUIDv4AndUnique(t *testing.T) {
	seen := make(map[string]struct{}, 4096)
	for i := 0; i < 4096; i++ {
		id := newEventID()
		if !uuidV4Re.MatchString(id) {
			t.Fatalf("id %q bukan UUID v4 kanonik", id)
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("id berulang setelah %d pembuatan: %q", i, id)
		}
		seen[id] = struct{}{}
	}
}
