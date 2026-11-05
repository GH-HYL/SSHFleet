package execute

import "testing"

// 两档报文逐字钉住：连上之前与连上之后，排查时问的不是同一件事。
//
// 期望值抄老版 `internal/ssh/shape_test.go` 那两条字面量，不由被测代码算。
func TestAbortMessage(t *testing.T) {
	if got, want := abortMessage(false), "sshfleet.abort.connect_canceled: interrupted while connecting"; got != want {
		t.Fatalf("还没连上那一档 want %q，got %q", want, got)
	}
	if got, want := abortMessage(true), "sshfleet.abort.canceled: interrupted after session established"; got != want {
		t.Fatalf("已连上那一档 want %q，got %q", want, got)
	}
}
