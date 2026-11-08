package domain

import "time"

type SyncState string

const (
	SyncDeviceOnly SyncState = "device_only"
	SyncSynced     SyncState = "synced"
)

type TaskStatus string

const (
	TaskActive    TaskStatus = "active"
	TaskCompleted TaskStatus = "completed"
)

type StageProgress string

const (
	StagePending StageProgress = "pending"
	StageDone    StageProgress = "done"
	StageSkipped StageProgress = "skipped"
)

type RedeemState string

const (
	RedeemUnrequested  RedeemState = "unrequested"
	RedeemPending      RedeemState = "pending"
	RedeemApproved     RedeemState = "approved"
	RedeemRejected     RedeemState = "rejected"
	RedeemSelfRedeemed RedeemState = "self_redeemed"
)

type VerificationStatus string

const (
	VerifPending   VerificationStatus = "pending"
	VerifApproved  VerificationStatus = "approved"
	VerifRejected  VerificationStatus = "rejected"
	VerifWithdrawn VerificationStatus = "withdrawn"
)

type RewardScope string

const (
	RewardStage          RewardScope = "stage"
	RewardTaskCompletion RewardScope = "task_completion"
)

type Account struct {
	ID       string
	Nickname string
	// AppleSubject and GoogleSubject are independent provider keys.
	// An empty value means that provider is not attached. Email is not an identity key
	// and must not merge these accounts.
	AppleSubject  string
	GoogleSubject string
	InviteCode    string
	CreatedAt     time.Time
}

type Task struct {
	ID                          string
	OwnerAccountID              string
	SyncState                   SyncState
	Title                       string
	Notes                       string
	Status                      TaskStatus
	DesignatedVerifierAccountID *string
	DueAt                       time.Time
	CreatedAt                   time.Time
	UpdatedAt                   time.Time
	CompletedAt                 *time.Time
	UploadConsentAt             *time.Time
}

type Stage struct {
	ID        string
	TaskID    string
	Position  int
	Title     string
	Progress  StageProgress
	RemindAt  *time.Time
	DoneAt    *time.Time
	SkippedAt *time.Time
}

type Reward struct {
	ID          string
	Scope       RewardScope
	TaskID      string
	StageID     *string
	Text        string
	Link        *string
	RedeemState RedeemState
	RedeemedAt  *time.Time
}

type VerificationRequest struct {
	ID                 string
	RewardID           string
	Attempt            int
	RequesterAccountID string
	VerifierAccountID  string
	Status             VerificationStatus
	Note               string
	ResolutionNote     string
	CreatedAt          time.Time
	ResolvedAt         *time.Time
}

type NoticeKind string

const (
	NoticeVerificationPending   NoticeKind = "verification_pending"
	NoticeVerificationApproved  NoticeKind = "verification_approved"
	NoticeVerificationRejected  NoticeKind = "verification_rejected"
	NoticeVerificationWithdrawn NoticeKind = "verification_withdrawn"
)

// InAppNotice is the MVP in-app verification result channel (PRD §10.11 / FR-26).
type InAppNotice struct {
	ID        string
	AccountID string
	Kind      NoticeKind
	RequestID string
	CreatedAt time.Time
	ReadAt    *time.Time
}

type ReportReason string

const (
	ReportHarassment     ReportReason = "harassment"      // 骚扰
	ReportPrivacy        ReportReason = "privacy"         // 隐私
	ReportIllegalContent ReportReason = "illegal_content" // 违法内容
	ReportOther          ReportReason = "other"           // 其他
)

func ParseReportReason(s string) (ReportReason, bool) {
	switch ReportReason(s) {
	case ReportHarassment, ReportPrivacy, ReportIllegalContent, ReportOther:
		return ReportReason(s), true
	default:
		return "", false
	}
}

// Report is a user complaint about a verification request's note/screenshot (PRD §10.12 / FR-27).
type Report struct {
	ID                string
	ReporterAccountID string
	RequestID         string
	Reason            ReportReason
	Detail            string
	CreatedAt         time.Time
}
