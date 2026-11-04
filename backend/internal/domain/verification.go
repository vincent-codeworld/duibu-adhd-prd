package domain

import "fmt"

// ApplyVerificationEvent mutates reward redeem state per PRD §9.
// Stage progress is never changed here.
func ApplyVerificationEvent(reward *Reward, event string) error {
	if reward == nil {
		return fmt.Errorf("nil reward")
	}
	switch event {
	case "submit":
		if reward.RedeemState != RedeemUnrequested && reward.RedeemState != RedeemRejected {
			return fmt.Errorf("cannot submit from %s", reward.RedeemState)
		}
		reward.RedeemState = RedeemPending
	case "approve":
		if reward.RedeemState != RedeemPending {
			return fmt.Errorf("cannot approve from %s", reward.RedeemState)
		}
		reward.RedeemState = RedeemApproved
	case "reject":
		if reward.RedeemState != RedeemPending {
			return fmt.Errorf("cannot reject from %s", reward.RedeemState)
		}
		reward.RedeemState = RedeemRejected
	case "withdraw":
		if reward.RedeemState != RedeemPending {
			return fmt.Errorf("cannot withdraw from %s", reward.RedeemState)
		}
		reward.RedeemState = RedeemUnrequested
	case "self_redeem":
		if reward.RedeemState != RedeemUnrequested && reward.RedeemState != RedeemRejected {
			return fmt.Errorf("cannot self-redeem from %s", reward.RedeemState)
		}
		reward.RedeemState = RedeemSelfRedeemed
	default:
		return fmt.Errorf("unknown event %q", event)
	}
	return nil
}

// CanUncheckStage is false when reward is locked by verification/self-redeem (PRD FR-05).
func CanUncheckStage(progress StageProgress, reward *Reward) bool {
	if progress != StageDone {
		return false
	}
	if reward == nil || reward.Text == "" {
		return true
	}
	switch reward.RedeemState {
	case RedeemUnrequested:
		return true
	default:
		return false
	}
}
