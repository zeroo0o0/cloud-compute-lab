package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"battleworld/cluster"
	"battleworld/protocol"
)

const (
	usersKey    = "users.json"
	sessionsKey = "sessions.json"
)

type KubeStore struct {
	client *cluster.Client
	name   string
	labels map[string]string
}

func NewKubeStore(client *cluster.Client, name string) *KubeStore {
	return &KubeStore{
		client: client,
		name:   name,
		labels: map[string]string{"app.kubernetes.io/part-of": "lab4", "lab4/state": "coordinator"},
	}
}

func (s *KubeStore) Register(username, password string) error {
	if username == "" || password == "" {
		return errors.New("用户名和密码不能为空")
	}
	return s.update(func(users map[string]protocol.UserProfile, sessions map[string]protocol.HotSession) error {
		if _, ok := users[username]; ok {
			return fmt.Errorf("用户 %q 已存在", username)
		}
		users[username] = protocol.UserProfile{
			Username:     username,
			PasswordHash: hashPassword(password),
			LastMap:      "green",
			X:            4,
			Y:            4,
			HP:           protocol.InitHP,
			MaxHP:        protocol.InitHP,
			Attack:       protocol.InitAttack,
			Potions:      protocol.MaxPotions,
			Alive:        true,
		}
		return nil
	})
}

func (s *KubeStore) Authenticate(username, password string) (*protocol.UserProfile, error) {
	users, _, err := s.load()
	if err != nil {
		return nil, err
	}
	if username == "" || password == "" {
		return nil, errors.New("用户名和密码不能为空")
	}
	user, ok := users[username]
	if !ok {
		return nil, fmt.Errorf("用户 %q 不存在", username)
	}
	if user.PasswordHash != hashPassword(password) {
		return nil, errors.New("密码错误")
	}
	copyUser := user
	return &copyUser, nil
}

func (s *KubeStore) LoadProfile(username string) (*protocol.UserProfile, error) {
	users, _, err := s.load()
	if err != nil {
		return nil, err
	}
	user, ok := users[username]
	if !ok {
		return nil, fmt.Errorf("用户 %q 不存在", username)
	}
	copyUser := user
	return &copyUser, nil
}

func (s *KubeStore) SaveProfile(profile protocol.UserProfile) error {
	return s.update(func(users map[string]protocol.UserProfile, sessions map[string]protocol.HotSession) error {
		existing, ok := users[profile.Username]
		if ok && profile.PasswordHash == "" {
			profile.PasswordHash = existing.PasswordHash
		}
		users[profile.Username] = profile
		return nil
	})
}

func (s *KubeStore) SaveHotSession(session protocol.HotSession) error {
	if session.UpdatedAt.IsZero() {
		session.UpdatedAt = time.Now()
	}
	return s.update(func(users map[string]protocol.UserProfile, sessions map[string]protocol.HotSession) error {
		sessions[session.Username] = session
		return nil
	})
}

func (s *KubeStore) LoadHotSession(username string) (*protocol.HotSession, bool, error) {
	_, sessions, err := s.load()
	if err != nil {
		return nil, false, err
	}
	session, ok := sessions[username]
	if !ok {
		return nil, false, nil
	}
	copySession := session
	return &copySession, true, nil
}

func (s *KubeStore) DeleteHotSession(username string) error {
	return s.update(func(users map[string]protocol.UserProfile, sessions map[string]protocol.HotSession) error {
		delete(sessions, username)
		return nil
	})
}

func (s *KubeStore) load() (map[string]protocol.UserProfile, map[string]protocol.HotSession, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cm, exists, err := s.client.GetConfigMap(ctx, s.name)
	if err != nil {
		return nil, nil, err
	}
	users := map[string]protocol.UserProfile{}
	sessions := map[string]protocol.HotSession{}
	if !exists {
		return users, sessions, nil
	}
	if raw := cm.Data[usersKey]; raw != "" {
		if err := json.Unmarshal([]byte(raw), &users); err != nil {
			return nil, nil, err
		}
	}
	if raw := cm.Data[sessionsKey]; raw != "" {
		if err := json.Unmarshal([]byte(raw), &sessions); err != nil {
			return nil, nil, err
		}
	}
	return users, sessions, nil
}

func (s *KubeStore) update(mutator func(map[string]protocol.UserProfile, map[string]protocol.HotSession) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	return s.client.UpdateConfigMapData(ctx, s.name, s.labels, func(data map[string]string) error {
		users := map[string]protocol.UserProfile{}
		sessions := map[string]protocol.HotSession{}
		if raw := data[usersKey]; raw != "" {
			if err := json.Unmarshal([]byte(raw), &users); err != nil {
				return err
			}
		}
		if raw := data[sessionsKey]; raw != "" {
			if err := json.Unmarshal([]byte(raw), &sessions); err != nil {
				return err
			}
		}
		if err := mutator(users, sessions); err != nil {
			return err
		}
		usersJSON, err := json.Marshal(users)
		if err != nil {
			return err
		}
		sessionsJSON, err := json.Marshal(sessions)
		if err != nil {
			return err
		}
		data[usersKey] = string(usersJSON)
		data[sessionsKey] = string(sessionsJSON)
		return nil
	})
}
