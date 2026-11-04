package store

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/vincent-codeworld/duibu-adhd/backend/internal/domain"
)

type Friendship struct {
	ID        string     `json:"id"`
	AccountA  string     `json:"accountA"`
	AccountB  string     `json:"accountB"`
	CreatedAt time.Time  `json:"createdAt"`
	RemovedAt *time.Time `json:"removedAt,omitempty"`
}

type FriendListItem struct {
	FriendshipID string    `json:"friendshipId"`
	AccountID    string    `json:"accountId"`
	Nickname     string    `json:"nickname"`
	Since        time.Time `json:"since"`
}

// Memory is an in-memory store for stub auth / early联调. Not for production.
type Memory struct {
	mu                sync.RWMutex
	byID              map[string]*domain.Account
	byApple           map[string]*domain.Account
	byInvite          map[string]*domain.Account
	friendships       map[string]*Friendship          // id -> friendship
	blocks            map[string]map[string]time.Time // blocker -> blocked -> at
	tasks             map[string]*memTask
	verifications     map[string]*domain.VerificationRequest
	screenshots       map[string]string               // requestID -> storageKey
	screenshotUploads map[string]*memScreenshotUpload // key -> upload meta
	notices           map[string]*domain.InAppNotice
	reports           map[string]*domain.Report
	inviteFails       map[string][]time.Time // accountID -> fail timestamps (process-local)
}

func NewMemory() *Memory {
	return &Memory{
		byID:              map[string]*domain.Account{},
		byApple:           map[string]*domain.Account{},
		byInvite:          map[string]*domain.Account{},
		friendships:       map[string]*Friendship{},
		blocks:            map[string]map[string]time.Time{},
		tasks:             map[string]*memTask{},
		verifications:     map[string]*domain.VerificationRequest{},
		screenshots:       map[string]string{},
		screenshotUploads: map[string]*memScreenshotUpload{},
		notices:           map[string]*domain.InAppNotice{},
		reports:           map[string]*domain.Report{},
		inviteFails:       map[string][]time.Time{},
	}
}

func (m *Memory) UpsertByAppleSubject(appleSubject, nickname string) (*domain.Account, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if a, ok := m.byApple[appleSubject]; ok {
		if nickname != "" && a.Nickname != nickname {
			a.Nickname = nickname
		}
		return cloneAccount(a), false, nil
	}
	if nickname == "" {
		nickname = "用户"
	}
	if len([]rune(nickname)) > 20 {
		return nil, false, fmt.Errorf("nickname too long")
	}
	code, err := randomInviteCode()
	if err != nil {
		return nil, false, err
	}
	a := &domain.Account{
		ID:           uuid.NewString(),
		Nickname:     nickname,
		AppleSubject: appleSubject,
		InviteCode:   code,
		CreatedAt:    time.Now().UTC(),
	}
	m.byID[a.ID] = a
	m.byApple[appleSubject] = a
	m.byInvite[code] = a
	return cloneAccount(a), true, nil
}

func (m *Memory) GetByID(id string) (*domain.Account, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	a, ok := m.byID[id]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return cloneAccount(a), nil
}

func (m *Memory) GetByInviteCode(code string) (*domain.Account, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	a, ok := m.byInvite[code]
	if !ok {
		return nil, fmt.Errorf("invalid invite code")
	}
	return cloneAccount(a), nil
}

func (m *Memory) UpdateNickname(id, nickname string) (*domain.Account, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.byID[id]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	n := len([]rune(nickname))
	if n < 1 || n > 20 {
		return nil, fmt.Errorf("nickname must be 1–20 characters")
	}
	a.Nickname = nickname
	return cloneAccount(a), nil
}

func (m *Memory) ResetInviteCode(id string) (*domain.Account, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.byID[id]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	delete(m.byInvite, a.InviteCode)
	code, err := randomInviteCode()
	if err != nil {
		return nil, err
	}
	a.InviteCode = code
	m.byInvite[code] = a
	return cloneAccount(a), nil
}

func (m *Memory) IsBlockedEitherWay(a, b string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.blocks[a] != nil {
		if _, ok := m.blocks[a][b]; ok {
			return true
		}
	}
	if m.blocks[b] != nil {
		if _, ok := m.blocks[b][a]; ok {
			return true
		}
	}
	return false
}

func (m *Memory) Block(blockerID, blockedID string) error {
	if blockerID == blockedID {
		return fmt.Errorf("cannot block yourself")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.byID[blockedID]; !ok {
		return fmt.Errorf("account not found")
	}
	if m.blocks[blockerID] == nil {
		m.blocks[blockerID] = map[string]time.Time{}
	}
	m.blocks[blockerID][blockedID] = time.Now().UTC()
	// soft-remove any active friendship
	for _, f := range m.friendships {
		if f.RemovedAt != nil {
			continue
		}
		if (f.AccountA == blockerID && f.AccountB == blockedID) || (f.AccountA == blockedID && f.AccountB == blockerID) {
			now := time.Now().UTC()
			f.RemovedAt = &now
		}
	}
	return nil
}

func (m *Memory) ListFriends(accountID string) ([]FriendListItem, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.byID[accountID]; !ok {
		return nil, fmt.Errorf("not found")
	}
	out := []FriendListItem{}
	for _, f := range m.friendships {
		if f.RemovedAt != nil {
			continue
		}
		var other string
		switch accountID {
		case f.AccountA:
			other = f.AccountB
		case f.AccountB:
			other = f.AccountA
		default:
			continue
		}
		acc := m.byID[other]
		if acc == nil {
			continue
		}
		out = append(out, FriendListItem{
			FriendshipID: f.ID,
			AccountID:    other,
			Nickname:     acc.Nickname,
			Since:        f.CreatedAt,
		})
	}
	return out, nil
}

// AddFriendByInvite implements PRD FR-15.
// Returns (item, alreadyFriends, error).
func (m *Memory) AddFriendByInvite(requesterID, inviteCode string) (FriendListItem, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	me, ok := m.byID[requesterID]
	if !ok {
		return FriendListItem{}, false, fmt.Errorf("not found")
	}
	other, ok := m.byInvite[inviteCode]
	if !ok {
		return FriendListItem{}, false, fmt.Errorf("invalid invite code")
	}
	if other.ID == me.ID {
		return FriendListItem{}, false, fmt.Errorf("cannot add yourself")
	}
	if m.blocks[requesterID] != nil {
		if _, ok := m.blocks[requesterID][other.ID]; ok {
			return FriendListItem{}, false, fmt.Errorf("blocked")
		}
	}
	if m.blocks[other.ID] != nil {
		if _, ok := m.blocks[other.ID][requesterID]; ok {
			return FriendListItem{}, false, fmt.Errorf("blocked")
		}
	}
	// existing active friendship?
	for _, f := range m.friendships {
		if f.RemovedAt != nil {
			continue
		}
		if (f.AccountA == me.ID && f.AccountB == other.ID) || (f.AccountA == other.ID && f.AccountB == me.ID) {
			return FriendListItem{
				FriendshipID: f.ID,
				AccountID:    other.ID,
				Nickname:     other.Nickname,
				Since:        f.CreatedAt,
			}, true, nil
		}
	}
	f := &Friendship{
		ID:        uuid.NewString(),
		AccountA:  me.ID,
		AccountB:  other.ID,
		CreatedAt: time.Now().UTC(),
	}
	m.friendships[f.ID] = f
	return FriendListItem{
		FriendshipID: f.ID,
		AccountID:    other.ID,
		Nickname:     other.Nickname,
		Since:        f.CreatedAt,
	}, false, nil
}

func (m *Memory) RemoveFriendship(requesterID, friendshipID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	f, ok := m.friendships[friendshipID]
	if !ok || f.RemovedAt != nil {
		return fmt.Errorf("not found")
	}
	if f.AccountA != requesterID && f.AccountB != requesterID {
		return fmt.Errorf("forbidden")
	}
	now := time.Now().UTC()
	f.RemovedAt = &now
	return nil
}

func cloneAccount(a *domain.Account) *domain.Account {
	cp := *a
	return &cp
}

func randomInviteCode() (string, error) {
	b := make([]byte, 5)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (m *Memory) AreFriends(a, b string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, f := range m.friendships {
		if f.RemovedAt != nil {
			continue
		}
		if (f.AccountA == a && f.AccountB == b) || (f.AccountA == b && f.AccountB == a) {
			return true
		}
	}
	return false
}
