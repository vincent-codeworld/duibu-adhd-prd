package store

import "time"

func (m *Memory) RecordInviteCodeFailure(accountID string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cutoff := at.Add(-time.Hour)
	kept := m.inviteFails[accountID][:0]
	for _, ts := range m.inviteFails[accountID] {
		if ts.After(cutoff) {
			kept = append(kept, ts)
		}
	}
	m.inviteFails[accountID] = append(kept, at)
	return nil
}

func (m *Memory) CountInviteCodeFailures(accountID string, since time.Time) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	kept := m.inviteFails[accountID][:0]
	for _, ts := range m.inviteFails[accountID] {
		if ts.After(since) {
			kept = append(kept, ts)
		}
	}
	if len(kept) == 0 {
		delete(m.inviteFails, accountID)
		return 0, nil
	}
	m.inviteFails[accountID] = kept
	return len(kept), nil
}
