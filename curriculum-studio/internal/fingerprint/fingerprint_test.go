package fingerprint

import "testing"

func TestHashCanonicalizesObjectKeyOrder(t *testing.T) {
	a, err := Hash([]byte(`{"b":2,"a":{"d":4,"c":3}}`))
	if err != nil {
		t.Fatal(err)
	}
	b, err := Hash([]byte(`{"a":{"c":3,"d":4},"b":2}`))
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatalf("equivalent JSON produced different fingerprints: %s != %s", a, b)
	}
}
