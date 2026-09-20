package smb2

import (
	"context"
	"testing"

	. "github.com/hirochachacha/go-smb2/internal/smb2"
)

// TestAccountLoanGrantsPartialCreditUnderContention exercises the exact
// mechanism behind a real-world bug: a caller (e.g. File.readdir) that
// requests a large multi-credit charge but only has a smaller balance
// available must be told how much it actually got, not just handed
// isComplete=false and left to assume the full charge went through.
func TestAccountLoanGrantsPartialCreditUnderContention(t *testing.T) {
	a := openAccount(8)
	// openAccount seeds one credit; drain it and grant only 3 more so the
	// balance holds exactly 3 available credits.
	<-a.balance
	a.charge(3, 3)

	got, isComplete, err := a.loan(5, context.Background())
	if err != nil {
		t.Fatalf("loan: %v", err)
	}
	if isComplete {
		t.Fatalf("expected a partial grant (only 3 of 5 credits available), got isComplete=true")
	}
	if got != 3 {
		t.Fatalf("expected a grant of 3 credits, got %d", got)
	}
}

func TestAccountLoanGrantsFullRequestWhenAvailable(t *testing.T) {
	a := openAccount(8)
	<-a.balance
	a.charge(5, 5)

	got, isComplete, err := a.loan(5, context.Background())
	if err != nil {
		t.Fatalf("loan: %v", err)
	}
	if !isComplete {
		t.Fatalf("expected a complete grant, got isComplete=false (got %d credits)", got)
	}
	if got != 5 {
		t.Fatalf("expected a grant of 5 credits, got %d", got)
	}
}

// TestConnLoanCreditShrinksGrantedPayloadOnPartialGrant is the invariant
// File.readdir/ioctl/queryInfo must honor: when large-MTU credits are
// scarce, the payload size a caller is entitled to declare on the wire
// (e.g. QueryDirectoryRequest.OutputBufferLength) must shrink along with
// the credit charge actually granted — declaring the original, larger
// size while only paying for a smaller charge is what a compliant server
// rejects with STATUS_INVALID_PARAMETER.
func TestConnLoanCreditShrinksGrantedPayloadOnPartialGrant(t *testing.T) {
	c := &conn{
		capabilities: SMB2_GLOBAL_CAP_LARGE_MTU,
		account:      openAccount(8),
	}
	// Seed exactly 2 credits available (1 from openAccount + 1 charged).
	<-c.account.balance
	c.account.charge(2, 2)

	requestedPayload := 256 * 1024 // needs 4 credits at 64KiB/credit
	creditCharge, grantedPayloadSize, err := c.loanCredit(requestedPayload, context.Background())
	if err != nil {
		t.Fatalf("loanCredit: %v", err)
	}
	if creditCharge != 2 {
		t.Fatalf("expected a partial charge of 2 credits, got %d", creditCharge)
	}
	if grantedPayloadSize != 2*64*1024 {
		t.Fatalf("expected granted payload capped to 2 credits worth (%d), got %d", 2*64*1024, grantedPayloadSize)
	}
	if grantedPayloadSize >= requestedPayload {
		t.Fatalf("granted payload (%d) should be smaller than what was requested (%d) under a partial grant", grantedPayloadSize, requestedPayload)
	}
}
