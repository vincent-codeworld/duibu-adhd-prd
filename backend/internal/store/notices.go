package store

import (
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/vincent-codeworld/duibu-adhd/backend/internal/domain"
)

// NoticeView is the API payload for GET /v1/notices.
type NoticeView struct {
	ID        string     `json:"id"`
	Kind      string     `json:"kind"`
	RequestID string     `json:"requestId"`
	CreatedAt time.Time  `json:"createdAt"`
	ReadAt    *time.Time `json:"readAt,omitempty"`
}

func (m *Memory) addNoticeLocked(accountID string, kind domain.NoticeKind, requestID string) {
	n := &domain.InAppNotice{
		ID:        uuid.NewString(),
		AccountID: accountID,
		Kind:      kind,
		RequestID: requestID,
		CreatedAt: time.Now().UTC(),
	}
	m.notices[n.ID] = n
}

func (m *Memory) ListNotices(accountID string) ([]NoticeView, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.byID[accountID]; !ok {
		return nil, fmt.Errorf("not found")
	}
	out := make([]NoticeView, 0)
	for _, n := range m.notices {
		if n.AccountID != accountID {
			continue
		}
		out = append(out, NoticeView{
			ID:        n.ID,
			Kind:      string(n.Kind),
			RequestID: n.RequestID,
			CreatedAt: n.CreatedAt,
			ReadAt:    n.ReadAt,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

func (m *Memory) MarkNoticeRead(accountID, noticeID string) (*NoticeView, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n, ok := m.notices[noticeID]
	if !ok || n.AccountID != accountID {
		return nil, fmt.Errorf("not found")
	}
	if n.ReadAt == nil {
		now := time.Now().UTC()
		n.ReadAt = &now
	}
	return &NoticeView{
		ID:        n.ID,
		Kind:      string(n.Kind),
		RequestID: n.RequestID,
		CreatedAt: n.CreatedAt,
		ReadAt:    n.ReadAt,
	}, nil
}
