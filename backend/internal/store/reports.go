package store

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/vincent-codeworld/duibu-adhd/backend/internal/domain"
)

const maxReportDetailRunes = 200

// ReportView is the API success receipt for POST /v1/reports.
type ReportView struct {
	ID        string    `json:"id"`
	RequestID string    `json:"requestId"`
	Reason    string    `json:"reason"`
	Detail    string    `json:"detail,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	Message   string    `json:"message"` // PRD FR-27: 举报人看到「已收到」
}

type CreateReportInput struct {
	RequestID string
	Reason    string
	Detail    string
}

// CreateReport persists a thin report into the manual queue (memory).
// Reporter must be requester or verifier of the target verification request.
// Does not change stage progress or reward state (FR-27).
func (m *Memory) CreateReport(reporterID string, in CreateReportInput) (*ReportView, error) {
	reason, ok := domain.ParseReportReason(strings.TrimSpace(in.Reason))
	if !ok {
		return nil, fmt.Errorf("invalid reason")
	}
	requestID := strings.TrimSpace(in.RequestID)
	if requestID == "" {
		return nil, fmt.Errorf("requestId required")
	}
	detail := strings.TrimSpace(in.Detail)
	if utf8.RuneCountInString(detail) > maxReportDetailRunes {
		return nil, fmt.Errorf("detail too long")
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.byID[reporterID]; !ok {
		return nil, fmt.Errorf("not found")
	}
	req, ok := m.verifications[requestID]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	if reporterID != req.RequesterAccountID && reporterID != req.VerifierAccountID {
		return nil, fmt.Errorf("forbidden")
	}

	rep := &domain.Report{
		ID:                uuid.NewString(),
		ReporterAccountID: reporterID,
		RequestID:         requestID,
		Reason:            reason,
		Detail:            detail,
		CreatedAt:         time.Now().UTC(),
	}
	m.reports[rep.ID] = rep
	return &ReportView{
		ID:        rep.ID,
		RequestID: rep.RequestID,
		Reason:    string(rep.Reason),
		Detail:    rep.Detail,
		CreatedAt: rep.CreatedAt,
		Message:   "已收到",
	}, nil
}
