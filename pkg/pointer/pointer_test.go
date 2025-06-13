package pointer_test

import (
	"testing"

	"peertech.de/axion/pkg/pointer"
)

func TestRef(t *testing.T) {
	type T int

	val := T(0)
	ptr := pointer.To(val)
	if *ptr != val {
		t.Errorf("expected %d, got %d", val, *ptr)
	}

	val = T(1)
	ptr = pointer.To(val)
	if *ptr != val {
		t.Errorf("expected %d, got %d", val, *ptr)
	}
}

func TestDeref(t *testing.T) {
	type T int

	var val, def T = 1, 0

	out := pointer.Deref(&val, def)
	if out != val {
		t.Errorf("expected %d, got %d", val, out)
	}

	out = pointer.Deref(nil, def)
	if out != def {
		t.Errorf("expected %d, got %d", def, out)
	}
}

func TestEqual(t *testing.T) {
	type T int

	if !pointer.Equal[T](nil, nil) {
		t.Errorf("expected true (nil == nil)")
	}
	if !pointer.Equal(pointer.To(T(123)), pointer.To(T(123))) {
		t.Errorf("expected true (val == val)")
	}
	if pointer.Equal(nil, pointer.To(T(123))) {
		t.Errorf("expected false (nil != val)")
	}
	if pointer.Equal(pointer.To(T(123)), nil) {
		t.Errorf("expected false (val != nil)")
	}
	if pointer.Equal(pointer.To(T(123)), pointer.To(T(456))) {
		t.Errorf("expected false (val != val)")
	}
}
