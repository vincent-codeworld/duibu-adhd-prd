package store

import (
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/vincent-codeworld/duibu-adhd/backend/internal/domain"
)

type StageInput struct {
	Title    string     `json:"title"`
	Progress string     `json:"progress,omitempty"`
	RemindAt *time.Time `json:"remindAt,omitempty"`
}

type RewardInput struct {
	Text string  `json:"text"`
	Link *string `json:"link,omitempty"`
}

type CreateTaskInput struct {
	Title                       string                 `json:"title"`
	Notes                       string                 `json:"notes"`
	UploadConsent               bool                   `json:"uploadConsent"`
	DueAt                       time.Time              `json:"dueAt"`
	Stages                      []StageInput           `json:"stages"`
	StageRewards                map[string]RewardInput `json:"stageRewards,omitempty"` // key = position as decimal string "1".."7"
	CompletionReward            *RewardInput           `json:"completionReward,omitempty"`
	DesignatedVerifierAccountID *string                `json:"designatedVerifierAccountId,omitempty"`
}

type PatchTaskInput struct {
	Title                 *string                `json:"title,omitempty"`
	Notes                 *string                `json:"notes,omitempty"`
	DueAt                 *time.Time             `json:"dueAt,omitempty"`
	Stages                []StageInput           `json:"stages,omitempty"`
	StageRewards          map[string]RewardInput `json:"stageRewards,omitempty"`
	CompletionReward      *RewardInput           `json:"completionReward,omitempty"`
	ClearCompletionReward bool                   `json:"clearCompletionReward,omitempty"`
}

type TaskView struct {
	ID                          string      `json:"id"`
	Title                       string      `json:"title"`
	Notes                       string      `json:"notes"`
	Status                      string      `json:"status"`
	DesignatedVerifierAccountID *string     `json:"designatedVerifierAccountId,omitempty"`
	DueAt                       time.Time   `json:"dueAt"`
	UploadConsentAt             time.Time   `json:"uploadConsentAt"`
	CreatedAt                   time.Time   `json:"createdAt"`
	UpdatedAt                   time.Time   `json:"updatedAt"`
	CompletedAt                 *time.Time  `json:"completedAt,omitempty"`
	Stages                      []StageView `json:"stages"`
	CompletionReward            *RewardView `json:"completionReward,omitempty"`
}

type StageView struct {
	ID       string      `json:"id"`
	Position int         `json:"position"`
	Title    string      `json:"title"`
	Progress string      `json:"progress"`
	RemindAt *time.Time  `json:"remindAt,omitempty"`
	Reward   *RewardView `json:"reward,omitempty"`
}

type RewardView struct {
	ID          string  `json:"id"`
	Text        string  `json:"text"`
	Link        *string `json:"link,omitempty"`
	RedeemState string  `json:"redeemState"`
}

type memTask struct {
	task    domain.Task
	stages  []domain.Stage
	rewards []domain.Reward
}

func validateTitleNotes(title, notes string) error {
	title = strings.TrimSpace(title)
	n := utf8.RuneCountInString(title)
	if n < 1 || n > 40 {
		return fmt.Errorf("title must be 1–40 characters")
	}
	if utf8.RuneCountInString(notes) > 500 {
		return fmt.Errorf("notes must be ≤500 characters")
	}
	return nil
}

func validateLink(link *string) error {
	if link == nil || strings.TrimSpace(*link) == "" {
		return nil
	}
	u := strings.TrimSpace(*link)
	if utf8.RuneCountInString(u) > 500 {
		return fmt.Errorf("link too long")
	}
	parsed, err := url.Parse(u)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("link must be http or https")
	}
	return nil
}

func validateStages(stages []StageInput) error {
	if len(stages) < 3 || len(stages) > 7 {
		return fmt.Errorf("need 3–7 stages")
	}
	for i, s := range stages {
		t := strings.TrimSpace(s.Title)
		if t == "" || utf8.RuneCountInString(t) > 40 {
			return fmt.Errorf("stage %d title invalid", i+1)
		}
		p := s.Progress
		if p == "" {
			p = string(domain.StagePending)
		}
		switch domain.StageProgress(p) {
		case domain.StagePending, domain.StageDone, domain.StageSkipped:
		default:
			return fmt.Errorf("stage %d progress invalid", i+1)
		}
	}
	return nil
}

func validateRewardText(text string) error {
	text = strings.TrimSpace(text)
	n := utf8.RuneCountInString(text)
	if n < 1 || n > 80 {
		return fmt.Errorf("reward text must be 1–80 characters")
	}
	return nil
}

func validateDueAt(dueAt time.Time) error {
	if dueAt.IsZero() {
		return fmt.Errorf("dueAt required")
	}
	return nil
}

func validateRemindAts(dueAt time.Time, stages []StageInput) error {
	for i, s := range stages {
		if s.RemindAt == nil {
			continue
		}
		if s.RemindAt.After(dueAt) {
			return fmt.Errorf("stage %d remindAt must be ≤ dueAt", i+1)
		}
	}
	return nil
}

func stagesReadyForCompletion(stages []domain.Stage) error {
	done := 0
	for _, s := range stages {
		if s.Progress == domain.StagePending {
			return fmt.Errorf("pending stages remain")
		}
		if s.Progress == domain.StageDone {
			done++
		}
	}
	if done < 1 {
		return fmt.Errorf("need at least one done stage")
	}
	return nil
}

const defaultCompletionGateText = "Task completion"

func (m *Memory) CreateSyncedTask(ownerID string, in CreateTaskInput) (*TaskView, error) {
	if !in.UploadConsent {
		return nil, fmt.Errorf("uploadConsent required")
	}
	if err := validateTitleNotes(in.Title, in.Notes); err != nil {
		return nil, err
	}
	if err := validateDueAt(in.DueAt); err != nil {
		return nil, err
	}
	if err := validateStages(in.Stages); err != nil {
		return nil, err
	}
	if err := validateRemindAts(in.DueAt.UTC(), in.Stages); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.byID[ownerID]; !ok {
		return nil, fmt.Errorf("unauthorized")
	}
	var verifier *string
	if in.DesignatedVerifierAccountID != nil && strings.TrimSpace(*in.DesignatedVerifierAccountID) != "" {
		vid := strings.TrimSpace(*in.DesignatedVerifierAccountID)
		if !m.areFriendsLocked(ownerID, vid) {
			return nil, fmt.Errorf("verifier must be a friend")
		}
		if m.isBlockedLocked(ownerID, vid) {
			return nil, fmt.Errorf("blocked")
		}
		verifier = &vid
	}
	now := time.Now().UTC()
	taskID := uuid.NewString()
	dueAt := in.DueAt.UTC()
	mt := &memTask{
		task: domain.Task{
			ID:                          taskID,
			OwnerAccountID:              ownerID,
			SyncState:                   domain.SyncSynced,
			Title:                       strings.TrimSpace(in.Title),
			Notes:                       strings.TrimSpace(in.Notes),
			Status:                      domain.TaskActive,
			DesignatedVerifierAccountID: verifier,
			DueAt:                       dueAt,
			CreatedAt:                   now,
			UpdatedAt:                   now,
			UploadConsentAt:             &now,
		},
	}
	for i, s := range in.Stages {
		prog := s.Progress
		if prog == "" {
			prog = string(domain.StagePending)
		}
		var remind *time.Time
		if s.RemindAt != nil {
			t := s.RemindAt.UTC()
			remind = &t
		}
		st := domain.Stage{
			ID:       uuid.NewString(),
			TaskID:   taskID,
			Position: i + 1,
			Title:    strings.TrimSpace(s.Title),
			Progress: domain.StageProgress(prog),
			RemindAt: remind,
		}
		mt.stages = append(mt.stages, st)
		key := fmt.Sprintf("%d", i+1)
		if rw, ok := in.StageRewards[key]; ok {
			if err := validateRewardText(rw.Text); err != nil {
				return nil, err
			}
			if err := validateLink(rw.Link); err != nil {
				return nil, err
			}
			sid := st.ID
			mt.rewards = append(mt.rewards, domain.Reward{
				ID:          uuid.NewString(),
				Scope:       domain.RewardStage,
				TaskID:      taskID,
				StageID:     &sid,
				Text:        strings.TrimSpace(rw.Text),
				Link:        rw.Link,
				RedeemState: domain.RedeemUnrequested,
			})
		}
	}
	needCompletion := verifier != nil
	if in.CompletionReward != nil && strings.TrimSpace(in.CompletionReward.Text) != "" {
		if err := validateRewardText(in.CompletionReward.Text); err != nil {
			return nil, err
		}
		if err := validateLink(in.CompletionReward.Link); err != nil {
			return nil, err
		}
		mt.rewards = append(mt.rewards, domain.Reward{
			ID:          uuid.NewString(),
			Scope:       domain.RewardTaskCompletion,
			TaskID:      taskID,
			Text:        strings.TrimSpace(in.CompletionReward.Text),
			Link:        in.CompletionReward.Link,
			RedeemState: domain.RedeemUnrequested,
		})
		needCompletion = false
	}
	if needCompletion {
		mt.rewards = append(mt.rewards, domain.Reward{
			ID:          uuid.NewString(),
			Scope:       domain.RewardTaskCompletion,
			TaskID:      taskID,
			Text:        defaultCompletionGateText,
			RedeemState: domain.RedeemUnrequested,
		})
	}
	m.tasks[taskID] = mt
	return taskViewLocked(mt), nil
}

func (m *Memory) ListSyncedTasks(ownerID string) ([]TaskView, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []TaskView{}
	for _, mt := range m.tasks {
		if mt.task.OwnerAccountID != ownerID {
			continue
		}
		out = append(out, *taskViewLocked(mt))
	}
	return out, nil
}

func (m *Memory) GetSyncedTask(ownerID, taskID string) (*TaskView, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	mt, ok := m.tasks[taskID]
	if !ok || mt.task.OwnerAccountID != ownerID {
		return nil, fmt.Errorf("not found")
	}
	return taskViewLocked(mt), nil
}

func (m *Memory) DeleteSyncedTask(ownerID, taskID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	mt, ok := m.tasks[taskID]
	if !ok || mt.task.OwnerAccountID != ownerID {
		return fmt.Errorf("not found")
	}
	rewardIDs := map[string]struct{}{}
	for _, r := range mt.rewards {
		rewardIDs[r.ID] = struct{}{}
	}
	for id, req := range m.verifications {
		if _, ok := rewardIDs[req.RewardID]; !ok {
			continue
		}
		if key, ok := m.screenshots[req.ID]; ok {
			delete(m.screenshotUploads, key)
			delete(m.screenshots, req.ID)
		}
		delete(m.verifications, id)
		// drop notices tied to this request
		for nid, n := range m.notices {
			if n.RequestID == id {
				delete(m.notices, nid)
			}
		}
	}
	delete(m.tasks, taskID)
	return nil
}

func (m *Memory) SetVerifier(ownerID, taskID, verifierID string) (*TaskView, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	mt, ok := m.tasks[taskID]
	if !ok || mt.task.OwnerAccountID != ownerID {
		return nil, fmt.Errorf("not found")
	}
	if !m.areFriendsLocked(ownerID, verifierID) {
		return nil, fmt.Errorf("verifier must be a friend")
	}
	if m.isBlockedLocked(ownerID, verifierID) {
		return nil, fmt.Errorf("blocked")
	}
	mt.task.DesignatedVerifierAccountID = &verifierID
	hasCompletion := false
	for _, rw := range mt.rewards {
		if rw.Scope == domain.RewardTaskCompletion {
			hasCompletion = true
			break
		}
	}
	if !hasCompletion {
		mt.rewards = append(mt.rewards, domain.Reward{
			ID:          uuid.NewString(),
			Scope:       domain.RewardTaskCompletion,
			TaskID:      taskID,
			Text:        defaultCompletionGateText,
			RedeemState: domain.RedeemUnrequested,
		})
	}
	mt.task.UpdatedAt = time.Now().UTC()
	return taskViewLocked(mt), nil
}

func (m *Memory) CompleteTask(ownerID, taskID string) (*TaskView, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	mt, ok := m.tasks[taskID]
	if !ok || mt.task.OwnerAccountID != ownerID {
		return nil, fmt.Errorf("not found")
	}
	if mt.task.DesignatedVerifierAccountID != nil {
		return nil, fmt.Errorf("verification required")
	}
	if err := stagesReadyForCompletion(mt.stages); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	mt.task.Status = domain.TaskCompleted
	mt.task.CompletedAt = &now
	mt.task.UpdatedAt = now
	return taskViewLocked(mt), nil
}

func (m *Memory) areFriendsLocked(a, b string) bool {
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

func (m *Memory) isBlockedLocked(a, b string) bool {
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

func taskViewLocked(mt *memTask) *TaskView {
	v := &TaskView{
		ID:                          mt.task.ID,
		Title:                       mt.task.Title,
		Notes:                       mt.task.Notes,
		Status:                      string(mt.task.Status),
		DesignatedVerifierAccountID: mt.task.DesignatedVerifierAccountID,
		DueAt:                       mt.task.DueAt,
		CreatedAt:                   mt.task.CreatedAt,
		UpdatedAt:                   mt.task.UpdatedAt,
		CompletedAt:                 mt.task.CompletedAt,
	}
	if mt.task.UploadConsentAt != nil {
		v.UploadConsentAt = *mt.task.UploadConsentAt
	}
	rewardByStage := map[string]*domain.Reward{}
	var completion *domain.Reward
	for i := range mt.rewards {
		rw := &mt.rewards[i]
		if rw.Scope == domain.RewardTaskCompletion {
			completion = rw
			continue
		}
		if rw.StageID != nil {
			rewardByStage[*rw.StageID] = rw
		}
	}
	for _, s := range mt.stages {
		sv := StageView{
			ID:       s.ID,
			Position: s.Position,
			Title:    s.Title,
			Progress: string(s.Progress),
			RemindAt: s.RemindAt,
		}
		if rw := rewardByStage[s.ID]; rw != nil {
			sv.Reward = &RewardView{ID: rw.ID, Text: rw.Text, Link: rw.Link, RedeemState: string(rw.RedeemState)}
		}
		v.Stages = append(v.Stages, sv)
	}
	if completion != nil {
		v.CompletionReward = &RewardView{ID: completion.ID, Text: completion.Text, Link: completion.Link, RedeemState: string(completion.RedeemState)}
	}
	return v
}

// PatchSyncedTask updates title/notes/stages/rewards for an owned synced task.
// Stage list if provided must remain 3–7 items; progress can be pending|done|skipped.
// FR-05: cannot uncheck (done→pending) when reward is pending/approved/rejected/self_redeemed.
func (m *Memory) PatchSyncedTask(ownerID, taskID string, in PatchTaskInput) (*TaskView, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	mt, ok := m.tasks[taskID]
	if !ok || mt.task.OwnerAccountID != ownerID {
		return nil, fmt.Errorf("not found")
	}
	if in.Title != nil {
		if err := validateTitleNotes(*in.Title, mt.task.Notes); err != nil {
			return nil, err
		}
		mt.task.Title = strings.TrimSpace(*in.Title)
	}
	if in.Notes != nil {
		if err := validateTitleNotes(mt.task.Title, *in.Notes); err != nil {
			return nil, err
		}
		mt.task.Notes = strings.TrimSpace(*in.Notes)
	}
	if in.DueAt != nil {
		if err := validateDueAt(*in.DueAt); err != nil {
			return nil, err
		}
		mt.task.DueAt = in.DueAt.UTC()
	}
	if in.Stages != nil {
		if err := validateStages(in.Stages); err != nil {
			return nil, err
		}
		if err := validateRemindAts(mt.task.DueAt, in.Stages); err != nil {
			return nil, err
		}
		// map old rewards by position
		oldByPos := map[int]*domain.Reward{}
		for i := range mt.rewards {
			rw := &mt.rewards[i]
			if rw.Scope != domain.RewardStage || rw.StageID == nil {
				continue
			}
			for _, s := range mt.stages {
				if s.ID == *rw.StageID {
					oldByPos[s.Position] = rw
					break
				}
			}
		}
		newStages := make([]domain.Stage, 0, len(in.Stages))
		newRewards := make([]domain.Reward, 0)
		// keep completion reward unless cleared/replaced
		for _, rw := range mt.rewards {
			if rw.Scope == domain.RewardTaskCompletion {
				newRewards = append(newRewards, rw)
			}
		}
		now := time.Now().UTC()
		for i, s := range in.Stages {
			pos := i + 1
			prog := s.Progress
			if prog == "" {
				prog = string(domain.StagePending)
			}
			var oldStage *domain.Stage
			for j := range mt.stages {
				if mt.stages[j].Position == pos {
					oldStage = &mt.stages[j]
					break
				}
			}
			stID := uuid.NewString()
			if oldStage != nil {
				stID = oldStage.ID
			}
			// FR-05 lock
			if oldStage != nil && oldStage.Progress == domain.StageDone && domain.StageProgress(prog) == domain.StagePending {
				if rw := oldByPos[pos]; rw != nil && !domain.CanUncheckStage(domain.StageDone, rw) {
					return nil, fmt.Errorf("cannot uncheck stage %d: reward locked", pos)
				}
			}
			var remind *time.Time
			if s.RemindAt != nil {
				t := s.RemindAt.UTC()
				remind = &t
			}
			st := domain.Stage{
				ID:       stID,
				TaskID:   taskID,
				Position: pos,
				Title:    strings.TrimSpace(s.Title),
				Progress: domain.StageProgress(prog),
				RemindAt: remind,
			}
			if st.Progress == domain.StageDone {
				st.DoneAt = &now
			}
			if st.Progress == domain.StageSkipped {
				st.SkippedAt = &now
			}
			newStages = append(newStages, st)

			if in.StageRewards != nil {
				key := fmt.Sprintf("%d", pos)
				if rwIn, ok := in.StageRewards[key]; ok {
					if err := validateRewardText(rwIn.Text); err != nil {
						return nil, err
					}
					if err := validateLink(rwIn.Link); err != nil {
						return nil, err
					}
					sid := st.ID
					state := domain.RedeemUnrequested
					id := uuid.NewString()
					if old := oldByPos[pos]; old != nil {
						id = old.ID
						state = old.RedeemState
					}
					newRewards = append(newRewards, domain.Reward{
						ID: id, Scope: domain.RewardStage, TaskID: taskID, StageID: &sid,
						Text: strings.TrimSpace(rwIn.Text), Link: rwIn.Link, RedeemState: state,
					})
				}
			} else if old := oldByPos[pos]; old != nil {
				sid := st.ID
				old.StageID = &sid
				newRewards = append(newRewards, *old)
			}
		}
		mt.stages = newStages
		mt.rewards = newRewards
	}
	if in.ClearCompletionReward {
		if mt.task.DesignatedVerifierAccountID != nil {
			return nil, fmt.Errorf("completion reward required when verifier designated")
		}
		filtered := mt.rewards[:0]
		for _, rw := range mt.rewards {
			if rw.Scope != domain.RewardTaskCompletion {
				filtered = append(filtered, rw)
			}
		}
		mt.rewards = filtered
	} else if in.CompletionReward != nil && strings.TrimSpace(in.CompletionReward.Text) != "" {
		if err := validateRewardText(in.CompletionReward.Text); err != nil {
			return nil, err
		}
		if err := validateLink(in.CompletionReward.Link); err != nil {
			return nil, err
		}
		found := false
		for i := range mt.rewards {
			if mt.rewards[i].Scope == domain.RewardTaskCompletion {
				mt.rewards[i].Text = strings.TrimSpace(in.CompletionReward.Text)
				mt.rewards[i].Link = in.CompletionReward.Link
				found = true
				break
			}
		}
		if !found {
			mt.rewards = append(mt.rewards, domain.Reward{
				ID: uuid.NewString(), Scope: domain.RewardTaskCompletion, TaskID: taskID,
				Text: strings.TrimSpace(in.CompletionReward.Text), Link: in.CompletionReward.Link,
				RedeemState: domain.RedeemUnrequested,
			})
		}
	}
	mt.task.UpdatedAt = time.Now().UTC()
	return taskViewLocked(mt), nil
}
