package shutdown

import "testing"

func TestCancelClosesDone(t *testing.T) {
	TestReset()
	defer TestReset()
	if Fired() {
		t.Fatal("should not be fired initially")
	}
	Cancel()
	Cancel() // idempotent — must not panic on double close
	if !Fired() {
		t.Fatal("should be fired after Cancel")
	}
	select {
	case <-Done():
	default:
		t.Fatal("Done() should be closed after Cancel")
	}
}

func TestContextCanceledOnCancel(t *testing.T) {
	TestReset()
	defer TestReset()
	ctx := Context()
	select {
	case <-ctx.Done():
		t.Fatal("context must not be done before Cancel")
	default:
	}
	Cancel()
	select {
	case <-ctx.Done():
	default:
		t.Fatal("context must be done after Cancel")
	}
}
