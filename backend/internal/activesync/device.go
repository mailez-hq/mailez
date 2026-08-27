package activesync

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"

	"mailez/backend/internal/core/models"
)

// syncSnapshot is the persisted per-collection item set used to compute
// incremental Sync adds/changes/deletes.
type syncSnapshot struct {
	UIDValidity uint32                  `json:"uid_validity"`
	Items       map[string]snapshotItem `json:"items"`
}

type snapshotItem struct {
	Read     bool `json:"read"`
	Flagged  bool `json:"flagged"`
	Answered bool `json:"answered"`
}

func emptySnapshot(uidValidity uint32) syncSnapshot {
	return syncSnapshot{UIDValidity: uidValidity, Items: map[string]snapshotItem{}}
}

func (s syncSnapshot) marshal() string {
	b, _ := json.Marshal(s)
	return string(b)
}

func unmarshalSnapshot(s string) (syncSnapshot, error) {
	var out syncSnapshot
	if s == "" {
		return emptySnapshot(0), nil
	}
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return out, err
	}
	if out.Items == nil {
		out.Items = map[string]snapshotItem{}
	}
	return out, nil
}

// registerDevice upserts the device row and refreshes the last-seen time.
func (s *Service) registerDevice(ctx context.Context, email, deviceID, deviceType, version, userAgent, policyKey string) error {
	if deviceID == "" {
		return errors.New("activesync: missing device id")
	}
	now := time.Now()
	var dev models.EasDevice
	err := s.DB.WithContext(ctx).
		Where("user_email = ? AND device_id = ?", email, deviceID).
		First(&dev).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		dev = models.EasDevice{
			UserEmail: email, DeviceID: deviceID, DeviceType: deviceType,
			ProtocolVersion: version, UserAgent: userAgent, PolicyKey: policyKey,
			LastSeenAt: now,
		}
		return s.DB.WithContext(ctx).Create(&dev).Error
	}
	if err != nil {
		return err
	}
	updates := map[string]any{
		"last_seen_at": now, "device_type": deviceType,
		"protocol_version": version, "user_agent": userAgent,
	}
	if policyKey != "" {
		updates["policy_key"] = policyKey
	}
	return s.DB.WithContext(ctx).Model(&models.EasDevice{}).
		Where("id = ?", dev.ID).Updates(updates).Error
}

// loadSyncState fetches the snapshot for a (device, collection) pair.
func (s *Service) loadSyncState(ctx context.Context, email, deviceID, collectionID string) (syncSnapshot, string, string, error) {
	var st models.EasSyncState
	err := s.DB.WithContext(ctx).
		Where("user_email = ? AND device_id = ? AND collection_id = ?", email, deviceID, collectionID).
		First(&st).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return emptySnapshot(0), "", "", nil
	}
	if err != nil {
		return emptySnapshot(0), "", "", err
	}
	snap, err := unmarshalSnapshot(st.Snapshot)
	return snap, st.SyncKey, st.ServerFolder, err
}

// saveSyncState persists a snapshot with a fresh sync key.
func (s *Service) saveSyncState(ctx context.Context, email, deviceID, collectionID, serverFolder string, snap syncSnapshot) (string, error) {
	key := newSyncKey()
	var st models.EasSyncState
	err := s.DB.WithContext(ctx).
		Where("user_email = ? AND device_id = ? AND collection_id = ?", email, deviceID, collectionID).
		First(&st).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		st = models.EasSyncState{
			UserEmail: email, DeviceID: deviceID, CollectionID: collectionID,
			SyncKey: key, ServerFolder: serverFolder, Snapshot: snap.marshal(),
			UIDValidity: snap.UIDValidity, UpdatedAt: time.Now(),
		}
		return key, s.DB.WithContext(ctx).Create(&st).Error
	}
	if err != nil {
		return "", err
	}
	st.SyncKey = key
	st.ServerFolder = serverFolder
	st.Snapshot = snap.marshal()
	st.UIDValidity = snap.UIDValidity
	st.UpdatedAt = time.Now()
	return key, s.DB.WithContext(ctx).Save(&st).Error
}

// pingState is the folder counter set the device last observed.
type pingState struct {
	UidNext  uint32
	Messages uint32
	Unseen   uint32
}

func (s *Service) loadPingState(ctx context.Context, email, deviceID, collectionID string) (pingState, error) {
	var st models.EasPingState
	err := s.DB.WithContext(ctx).
		Where("user_email = ? AND device_id = ? AND collection_id = ?", email, deviceID, collectionID).
		First(&st).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return pingState{}, nil
	}
	return pingState{UidNext: st.UidNext, Messages: st.Messages, Unseen: st.Unseen}, err
}

func (s *Service) savePingState(ctx context.Context, email, deviceID, collectionID string, st pingState) error {
	var row models.EasPingState
	err := s.DB.WithContext(ctx).
		Where("user_email = ? AND device_id = ? AND collection_id = ?", email, deviceID, collectionID).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return s.DB.WithContext(ctx).Create(&models.EasPingState{
			UserEmail: email, DeviceID: deviceID, CollectionID: collectionID,
			UidNext: st.UidNext, Messages: st.Messages, Unseen: st.Unseen,
			UpdatedAt: time.Now(),
		}).Error
	}
	if err != nil {
		return err
	}
	return s.DB.WithContext(ctx).Model(&models.EasPingState{}).
		Where("id = ?", row.ID).
		Updates(map[string]any{
			"uid_next": st.UidNext, "messages": st.Messages, "unseen": st.Unseen,
			"updated_at": time.Now(),
		}).Error
}

func newSyncKey() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "s" + hex.EncodeToString(b)
}
