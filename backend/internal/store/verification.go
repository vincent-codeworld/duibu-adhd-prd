package store

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/vincent-codeworld/duibu-adhd/backend/internal/domain"
)

type SubmitVerificationInput struct {
	Note          string `json:"note"`
	ScreenshotKey string `json:"screenshotKey,omitempty"` // from POST /v1/uploads/screenshots
}

// InboxItem is what a verifier sees — NEVER includes task notes (PRD FR-17).
type InboxItem struct {
	RequestID         string    `json:"requestId"`
	RewardID          string    `json:"rewardId"`
	Attempt           int       `json:"attempt"`
	Status            string    `json:"status"`
	Note              string    `json:"note"`
	ScreenshotKey     string    `json:"screenshotKey,omitempty"`
	ScreenshotURL     string    `json:"screenshotUrl,omitempty"`
	ApplicantNickname string    `json:"applicantNickname"`
	TaskTitle         string    `json:"taskTitle"`
	StageTitle        *string   `json:"stageTitle,omitempty"` // nil => task completion
	RewardText        string    `json:"rewardText"`
	RewardLink        *string   `json:"rewardLink,omitempty"`
	CreatedAt         time.Time `json:"createdAt"`
}

type OutboxItem struct {
	RequestID      string     `json:"requestId"`
	RewardID       string     `json:"rewardId"`
	Attempt        int        `json:"attempt"`
	Status         string     `json:"status"`
	Note           string     `json:"note"`
	ResolutionNote string     `json:"resolutionNote,omitempty"`
	CreatedAt      time.Time  `json:"createdAt"`
	ResolvedAt     *time.Time `json:"resolvedAt,omitempty"`
	TaskTitle      string     `json:"taskTitle"`
	StageTitle     *string    `json:"stageTitle,omitempty"`
	RewardText     string     `json:"rewardText"`
}

func (m *Memory) findRewardLocked(rewardID string) (*memTask, *domain.Reward, error) {
	for _, mt := range m.tasks {
		for i := range mt.rewards {
			if mt.rewards[i].ID == rewardID {
				return mt, &mt.rewards[i], nil
			}
		}
	}
	return nil, nil, fmt.Errorf("not found")
}

func (m *Memory) nextAttemptLocked(rewardID string) int {
	max := 0
	for _, v := range m.verifications {
		if v.RewardID == rewardID && v.Attempt > max {
			max = v.Attempt
		}
	}
	return max + 1
}

func (m *Memory) SubmitVerification(requesterID, rewardID string, in SubmitVerificationInput) (*InboxItem, error) {
	if utf8.RuneCountInString(in.Note) > 200 {
		return nil, fmt.Errorf("note too long")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	mt, rw, err := m.findRewardLocked(rewardID)
	if err != nil {
		return nil, err
	}
	if mt.task.OwnerAccountID != requesterID {
		return nil, fmt.Errorf("forbidden")
	}
	if mt.task.DesignatedVerifierAccountID == nil {
		return nil, fmt.Errorf("verifier not designated")
	}
	verifierID := *mt.task.DesignatedVerifierAccountID
	if !m.areFriendsLocked(requesterID, verifierID) {
		return nil, fmt.Errorf("verifier must be a friend")
	}
	if m.isBlockedLocked(requesterID, verifierID) {
		return nil, fmt.Errorf("blocked")
	}
	// stage must be done for stage rewards; completion requires completed task
	if rw.Scope == domain.RewardStage {
		if rw.StageID == nil {
			return nil, fmt.Errorf("invalid reward")
		}
		var st *domain.Stage
		for i := range mt.stages {
			if mt.stages[i].ID == *rw.StageID {
				st = &mt.stages[i]
				break
			}
		}
		if st == nil || st.Progress != domain.StageDone {
			return nil, fmt.Errorf("stage not done")
		}
		if st.Progress == domain.StageSkipped {
			return nil, fmt.Errorf("skipped stage cannot verify")
		}
	} else {
		// PRD v1.1: task_completion verification is the complete gate when a verifier is set.
		// Submit while task is still active once stages are ready; approval marks completed.
		if mt.task.Status == domain.TaskCompleted {
			// already completed — allow reward re-submit path only if redeem allows (rejected → submit)
		} else if err := stagesReadyForCompletion(mt.stages); err != nil {
			return nil, err
		}
	}
	shot := strings.TrimSpace(in.ScreenshotKey)
	if shot != "" {
		u, ok := m.screenshotUploads[shot]
		if !ok {
			return nil, fmt.Errorf("screenshot not found")
		}
		if u.UploaderAccountID != requesterID {
			return nil, fmt.Errorf("screenshot not owned")
		}
		if u.AttachedRequestID != nil {
			return nil, fmt.Errorf("screenshot already attached")
		}
	}
	if err := domain.ApplyVerificationEvent(rw, "submit"); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	req := &domain.VerificationRequest{
		ID:                 uuid.NewString(),
		RewardID:           rewardID,
		Attempt:            m.nextAttemptLocked(rewardID),
		RequesterAccountID: requesterID,
		VerifierAccountID:  verifierID,
		Status:             domain.VerifPending,
		Note:               strings.TrimSpace(in.Note),
		CreatedAt:          now,
	}
	m.verifications[req.ID] = req
	if err := m.attachScreenshotLocked(requesterID, shot, req.ID); err != nil {
		// Should not happen after pre-check; keep store consistent.
		delete(m.verifications, req.ID)
		_ = domain.ApplyVerificationEvent(rw, "withdraw")
		return nil, err
	}
	m.addNoticeLocked(verifierID, domain.NoticeVerificationPending, req.ID)
	return m.inboxItemLocked(req, mt, rw), nil
}

func (m *Memory) ListInbox(verifierID string) ([]InboxItem, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []InboxItem{}
	for _, req := range m.verifications {
		if req.VerifierAccountID != verifierID || req.Status != domain.VerifPending {
			continue
		}
		mt, rw, err := m.findRewardLocked(req.RewardID)
		if err != nil {
			continue
		}
		out = append(out, *m.inboxItemLocked(req, mt, rw))
	}
	return out, nil
}

func (m *Memory) ListOutbox(requesterID string) ([]OutboxItem, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []OutboxItem{}
	for _, req := range m.verifications {
		if req.RequesterAccountID != requesterID {
			continue
		}
		mt, rw, err := m.findRewardLocked(req.RewardID)
		if err != nil {
			continue
		}
		item := OutboxItem{
			RequestID:      req.ID,
			RewardID:       req.RewardID,
			Attempt:        req.Attempt,
			Status:         string(req.Status),
			Note:           req.Note,
			ResolutionNote: req.ResolutionNote,
			CreatedAt:      req.CreatedAt,
			ResolvedAt:     req.ResolvedAt,
			TaskTitle:      mt.task.Title,
			RewardText:     rw.Text,
		}
		if rw.Scope == domain.RewardStage && rw.StageID != nil {
			for _, s := range mt.stages {
				if s.ID == *rw.StageID {
					t := s.Title
					item.StageTitle = &t
					break
				}
			}
		}
		out = append(out, item)
	}
	return out, nil
}

func (m *Memory) Approve(verifierID, requestID string) error {
	return m.resolve(verifierID, requestID, "approve", "")
}

func (m *Memory) Reject(verifierID, requestID, resolutionNote string) error {
	if utf8.RuneCountInString(resolutionNote) > 80 {
		return fmt.Errorf("resolution note too long")
	}
	return m.resolve(verifierID, requestID, "reject", strings.TrimSpace(resolutionNote))
}

func (m *Memory) Withdraw(requesterID, requestID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	req, ok := m.verifications[requestID]
	if !ok {
		return fmt.Errorf("not found")
	}
	if req.RequesterAccountID != requesterID {
		return fmt.Errorf("forbidden")
	}
	if req.Status != domain.VerifPending {
		return fmt.Errorf("not pending")
	}
	_, rw, err := m.findRewardLocked(req.RewardID)
	if err != nil {
		return err
	}
	if err := domain.ApplyVerificationEvent(rw, "withdraw"); err != nil {
		return err
	}
	now := time.Now().UTC()
	req.Status = domain.VerifWithdrawn
	req.ResolvedAt = &now
	m.addNoticeLocked(req.VerifierAccountID, domain.NoticeVerificationWithdrawn, req.ID)
	return nil
}

func (m *Memory) resolve(verifierID, requestID, event, resolutionNote string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	req, ok := m.verifications[requestID]
	if !ok {
		return fmt.Errorf("not found")
	}
	if req.VerifierAccountID != verifierID {
		return fmt.Errorf("forbidden")
	}
	if req.Status != domain.VerifPending {
		return fmt.Errorf("not pending")
	}
	mt, rw, err := m.findRewardLocked(req.RewardID)
	if err != nil {
		return err
	}
	if err := domain.ApplyVerificationEvent(rw, event); err != nil {
		return err
	}
	now := time.Now().UTC()
	req.ResolvedAt = &now
	req.ResolutionNote = resolutionNote
	if event == "approve" {
		req.Status = domain.VerifApproved
		rw.RedeemedAt = &now
		m.addNoticeLocked(req.RequesterAccountID, domain.NoticeVerificationApproved, req.ID)
		if rw.Scope == domain.RewardTaskCompletion && mt.task.Status != domain.TaskCompleted {
			mt.task.Status = domain.TaskCompleted
			mt.task.CompletedAt = &now
			mt.task.UpdatedAt = now
		}
	} else {
		req.Status = domain.VerifRejected
		m.addNoticeLocked(req.RequesterAccountID, domain.NoticeVerificationRejected, req.ID)
	}
	return nil
}

func (m *Memory) inboxItemLocked(req *domain.VerificationRequest, mt *memTask, rw *domain.Reward) *InboxItem {
	applicant := m.byID[req.RequesterAccountID]
	nick := ""
	if applicant != nil {
		nick = applicant.Nickname
	}
	shotKey, shotURL := withScreenshotURL(m.screenshots[req.ID])
	item := &InboxItem{
		RequestID:         req.ID,
		RewardID:          req.RewardID,
		Attempt:           req.Attempt,
		Status:            string(req.Status),
		Note:              req.Note,
		ScreenshotKey:     shotKey,
		ScreenshotURL:     shotURL,
		ApplicantNickname: nick,
		TaskTitle:         mt.task.Title,
		RewardText:        rw.Text,
		RewardLink:        rw.Link,
		CreatedAt:         req.CreatedAt,
	}
	// deliberately omit mt.task.Notes
	if rw.Scope == domain.RewardStage && rw.StageID != nil {
		for _, s := range mt.stages {
			if s.ID == *rw.StageID {
				t := s.Title
				item.StageTitle = &t
				break
			}
		}
	}
	return item
}
