package store

import (
	"time"

	"github.com/vincent-codeworld/duibu-adhd/backend/internal/domain"
)

// Store is the persistence surface used by HTTP handlers.
// Memory is for unit tests / STORE_DRIVER=memory; Postgres is the default runtime path.
type Store interface {
	UpsertByAppleSubject(appleSubject, nickname string) (*domain.Account, bool, error)
	GetByID(id string) (*domain.Account, error)
	GetByInviteCode(code string) (*domain.Account, error)
	UpdateNickname(id, nickname string) (*domain.Account, error)
	ResetInviteCode(id string) (*domain.Account, error)

	IsBlockedEitherWay(a, b string) bool
	Block(blockerID, blockedID string) error
	ListFriends(accountID string) ([]FriendListItem, error)
	AddFriendByInvite(requesterID, inviteCode string) (FriendListItem, bool, error)
	RemoveFriendship(requesterID, friendshipID string) error
	AreFriends(a, b string) bool

	CreateSyncedTask(ownerID string, in CreateTaskInput) (*TaskView, error)
	ListSyncedTasks(ownerID string) ([]TaskView, error)
	GetSyncedTask(ownerID, taskID string) (*TaskView, error)
	DeleteSyncedTask(ownerID, taskID string) error
	SetVerifier(ownerID, taskID, verifierID string) (*TaskView, error)
	CompleteTask(ownerID, taskID string) (*TaskView, error)
	PatchSyncedTask(ownerID, taskID string, in PatchTaskInput) (*TaskView, error)

	SubmitVerification(requesterID, rewardID string, in SubmitVerificationInput) (*InboxItem, error)
	ListInbox(verifierID string) ([]InboxItem, error)
	ListOutbox(requesterID string) ([]OutboxItem, error)
	Approve(verifierID, requestID string) error
	Reject(verifierID, requestID, resolutionNote string) error
	Withdraw(requesterID, requestID string) error

	ListNotices(accountID string) ([]NoticeView, error)
	MarkNoticeRead(accountID, noticeID string) (*NoticeView, error)

	CreateReport(reporterID string, in CreateReportInput) (*ReportView, error)

	RegisterScreenshotUpload(uploaderID, key, contentType string, byteSize int) error
	GetScreenshotMeta(key string) (*ScreenshotUploadMeta, error)
	CanFetchScreenshot(viewerID, key string) (*ScreenshotUploadMeta, error)
	ScreenshotKeysForTask(ownerID, taskID string) ([]string, error)

	// Invite-code failure rate limit (rolling 1h). Postgres persists; Memory is process-local.
	RecordInviteCodeFailure(accountID string, at time.Time) error
	CountInviteCodeFailures(accountID string, since time.Time) (int, error)
}

// Ensure Memory implements Store.
var _ Store = (*Memory)(nil)
