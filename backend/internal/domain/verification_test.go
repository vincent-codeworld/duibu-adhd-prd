package domain

import "testing"

func TestApplyVerificationEvent_RejectKeepsRedeemRejected(t *testing.T) {
	r := &Reward{RedeemState: RedeemPending}
	if err := ApplyVerificationEvent(r, "reject"); err != nil {
		t.Fatal(err)
	}
	if r.RedeemState != RedeemRejected {
		t.Fatalf("got %s", r.RedeemState)
	}
}

func TestApplyVerificationEvent_CannotSubmitWhilePending(t *testing.T) {
	r := &Reward{RedeemState: RedeemPending}
	if err := ApplyVerificationEvent(r, "submit"); err == nil {
		t.Fatal("expected error")
	}
}

func TestCanUncheckStage_BlockedWhenPending(t *testing.T) {
	r := &Reward{Text: "奶茶", RedeemState: RedeemPending}
	if CanUncheckStage(StageDone, r) {
		t.Fatal("should block uncheck")
	}
}

func TestCanUncheckStage_AllowedWhenUnrequested(t *testing.T) {
	r := &Reward{Text: "奶茶", RedeemState: RedeemUnrequested}
	if !CanUncheckStage(StageDone, r) {
		t.Fatal("should allow uncheck")
	}
}
