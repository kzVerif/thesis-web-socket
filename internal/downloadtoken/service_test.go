package downloadtoken

import "testing"

func TestNewIsRandomAndHashed(t *testing.T) {
	a, ah, e := New()
	if e != nil {
		t.Fatal(e)
	}
	b, bh, e := New()
	if e != nil {
		t.Fatal(e)
	}
	if a == b || ah == bh {
		t.Fatal("tokens must be unique")
	}
	if Hash(a) != ah || len(ah) != 64 {
		t.Fatal("invalid hash")
	}
}
