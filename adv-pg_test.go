package advpg

import "testing"

func TestSelectOptions(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		opt := NewSelectOptions()
		if opt.Limit() != 0 {
			t.Errorf("Limit: got %d, want 0", opt.Limit())
		}
		if opt.Offset() != 0 {
			t.Errorf("Offset: got %d, want 0", opt.Offset())
		}
	})

	t.Run("WithLimit", func(t *testing.T) {
		opt := NewSelectOptions(WithLimit(42))
		if opt.Limit() != 42 {
			t.Errorf("Limit: got %d, want 42", opt.Limit())
		}
	})

	t.Run("WithOffset", func(t *testing.T) {
		opt := NewSelectOptions(WithOffset(7))
		if opt.Offset() != 7 {
			t.Errorf("Offset: got %d, want 7", opt.Offset())
		}
	})

	t.Run("combined", func(t *testing.T) {
		opt := NewSelectOptions(WithLimit(10), WithOffset(20))
		if opt.Limit() != 10 {
			t.Errorf("Limit: got %d, want 10", opt.Limit())
		}
		if opt.Offset() != 20 {
			t.Errorf("Offset: got %d, want 20", opt.Offset())
		}
	})
}
