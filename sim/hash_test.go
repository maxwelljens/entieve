package sim

import (
	"math"
	"strings"
	"testing"
)

// recoverHash returns the value Hash() panicked with, or nil when it
// returned normally.
func recoverHash(s *Sim) (recovered any) {
	defer func() { recovered = recover() }()
	s.Hash()
	return nil
}

// TestHashNeverDegradesToPartialFingerprint pins the no-fallback rule (F8):
// a state that cannot be serialized (a non-finite float in the Save) must
// not yield a plausible-looking hash built from seed/ticks/wallet — twin-run
// comparisons could then "agree" on two meaningless values. Hash() fails
// loudly instead, and a healthy sim still fingerprints normally.
func TestHashNeverDegradesToPartialFingerprint(t *testing.T) {
	s := newTestSim(t)
	healthy := s.Hash()
	if healthy == 0 {
		t.Fatal("healthy sim hashed to 0")
	}
	// The healthy fingerprint is state-sensitive (wallet included).
	s.wallet++
	if moved := s.Hash(); moved == healthy {
		t.Fatal("Hash() ignored a wallet change")
	}
	s.wallet--

	r := recoverHash(s)
	if r != nil {
		t.Fatalf("healthy state panicked: %v", r)
	}

	// Corrupt the state: JSON cannot encode NaN, so serialization fails.
	s.wallet = math.NaN()
	r = recoverHash(s)
	if r == nil {
		t.Fatal("Hash() returned a value for an unserializable state — " +
			"the partial-state fallback must be gone")
	}
	msg, ok := r.(string)
	if !ok || !strings.Contains(msg, "cannot fingerprint the state") {
		t.Fatalf("panic = %v, want the hash-failure message", r)
	}
}
