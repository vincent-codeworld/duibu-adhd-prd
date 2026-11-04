package store

import (
	"fmt"
	"strings"
	"time"
)

// ScreenshotUploadMeta is ownership metadata for a stored screenshot blob.
type ScreenshotUploadMeta struct {
	Key                string
	UploaderAccountID  string
	ContentType        string
	ByteSize           int
	CreatedAt          time.Time
	AttachedRequestID  *string
	RequesterAccountID string // set when attached (from verification_requests)
	VerifierAccountID  string // set when attached
}

func screenshotURL(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	return "/v1/screenshots/" + key
}

func withScreenshotURL(key string) (string, string) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", ""
	}
	return key, screenshotURL(key)
}

// --- Memory ---

type memScreenshotUpload struct {
	Key               string
	UploaderAccountID string
	ContentType       string
	ByteSize          int
	CreatedAt         time.Time
	AttachedRequestID *string
}

func (m *Memory) RegisterScreenshotUpload(uploaderID, key, contentType string, byteSize int) error {
	key = strings.TrimSpace(key)
	if key == "" || uploaderID == "" || byteSize <= 0 {
		return fmt.Errorf("invalid upload")
	}
	switch contentType {
	case "image/jpeg", "image/png", "image/webp":
	default:
		return fmt.Errorf("unsupported image type")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.byID[uploaderID]; !ok {
		return fmt.Errorf("not found")
	}
	if _, exists := m.screenshotUploads[key]; exists {
		return fmt.Errorf("conflict")
	}
	m.screenshotUploads[key] = &memScreenshotUpload{
		Key:               key,
		UploaderAccountID: uploaderID,
		ContentType:       contentType,
		ByteSize:          byteSize,
		CreatedAt:         time.Now().UTC(),
	}
	return nil
}

func (m *Memory) GetScreenshotMeta(key string) (*ScreenshotUploadMeta, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.screenshotMetaLocked(strings.TrimSpace(key))
}

func (m *Memory) screenshotMetaLocked(key string) (*ScreenshotUploadMeta, error) {
	u, ok := m.screenshotUploads[key]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	meta := &ScreenshotUploadMeta{
		Key:               u.Key,
		UploaderAccountID: u.UploaderAccountID,
		ContentType:       u.ContentType,
		ByteSize:          u.ByteSize,
		CreatedAt:         u.CreatedAt,
		AttachedRequestID: u.AttachedRequestID,
	}
	if u.AttachedRequestID != nil {
		if req, ok := m.verifications[*u.AttachedRequestID]; ok {
			meta.RequesterAccountID = req.RequesterAccountID
			meta.VerifierAccountID = req.VerifierAccountID
		}
	}
	return meta, nil
}

func (m *Memory) CanFetchScreenshot(viewerID, key string) (*ScreenshotUploadMeta, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	meta, err := m.screenshotMetaLocked(strings.TrimSpace(key))
	if err != nil {
		return nil, err
	}
	if meta.AttachedRequestID == nil {
		if meta.UploaderAccountID != viewerID {
			return nil, fmt.Errorf("forbidden")
		}
		return meta, nil
	}
	if viewerID == meta.RequesterAccountID || viewerID == meta.VerifierAccountID {
		return meta, nil
	}
	return nil, fmt.Errorf("forbidden")
}

func (m *Memory) ScreenshotKeysForTask(ownerID, taskID string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	mt, ok := m.tasks[taskID]
	if !ok || mt.task.OwnerAccountID != ownerID {
		return nil, fmt.Errorf("not found")
	}
	rewardIDs := map[string]struct{}{}
	for _, r := range mt.rewards {
		rewardIDs[r.ID] = struct{}{}
	}
	seen := map[string]struct{}{}
	var keys []string
	for _, req := range m.verifications {
		if _, ok := rewardIDs[req.RewardID]; !ok {
			continue
		}
		if k, ok := m.screenshots[req.ID]; ok && k != "" {
			if _, dup := seen[k]; !dup {
				seen[k] = struct{}{}
				keys = append(keys, k)
			}
		}
	}
	return keys, nil
}

func (m *Memory) attachScreenshotLocked(uploaderID, key, requestID string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil
	}
	u, ok := m.screenshotUploads[key]
	if !ok {
		return fmt.Errorf("screenshot not found")
	}
	if u.UploaderAccountID != uploaderID {
		return fmt.Errorf("screenshot not owned")
	}
	if u.AttachedRequestID != nil {
		return fmt.Errorf("screenshot already attached")
	}
	id := requestID
	u.AttachedRequestID = &id
	m.screenshots[requestID] = key
	return nil
}
