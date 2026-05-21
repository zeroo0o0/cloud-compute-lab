package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"battleworld/protocol"
	"battleworld/redisx"
)

type RedisStore struct {
	client *redisx.Client
	prefix string
}

func NewRedisStore(client *redisx.Client, prefix string) *RedisStore {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = "lab4"
	}
	return &RedisStore{client: client, prefix: prefix}
}

func (s *RedisStore) Register(username, password string) error {
	if username == "" || password == "" {
		return errors.New("用户名和密码不能为空")
	}
	profile := protocol.UserProfile{
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
	data, err := marshalJSON(profile)
	if err != nil {
		return err
	}
	ok, err := s.client.SetNXEX(context.Background(), s.userKey(username), data, 3650*24*time.Hour)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("用户 %q 已存在", username)
	}
	return nil
}

func (s *RedisStore) Authenticate(username, password string) (*protocol.UserProfile, error) {
	if username == "" || password == "" {
		return nil, errors.New("用户名和密码不能为空")
	}
	var profile protocol.UserProfile
	ok, err := s.client.GetJSON(context.Background(), s.userKey(username), &profile)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("用户 %q 不存在", username)
	}
	if profile.PasswordHash != hashPassword(password) {
		return nil, errors.New("密码错误")
	}
	return &profile, nil
}

func (s *RedisStore) LoadProfile(username string) (*protocol.UserProfile, error) {
	if username == "" {
		return nil, errors.New("用户名不能为空")
	}
	var profile protocol.UserProfile
	ok, err := s.client.GetJSON(context.Background(), s.userKey(username), &profile)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("用户 %q 不存在", username)
	}
	return &profile, nil
}

func (s *RedisStore) SaveProfile(profile protocol.UserProfile) error {
	if profile.Username == "" {
		return errors.New("用户名不能为空")
	}
	if profile.PasswordHash == "" {
		var existing protocol.UserProfile
		if ok, err := s.client.GetJSON(context.Background(), s.userKey(profile.Username), &existing); err != nil {
			return err
		} else if ok {
			profile.PasswordHash = existing.PasswordHash
		}
	}
	return s.client.SetJSON(context.Background(), s.userKey(profile.Username), profile)
}

func (s *RedisStore) SaveHotSession(session protocol.HotSession) error {
	if session.Username == "" {
		return nil
	}
	if session.UpdatedAt.IsZero() {
		session.UpdatedAt = time.Now()
	}
	return s.client.SetJSON(context.Background(), s.sessionKey(session.Username), session)
}

func (s *RedisStore) LoadHotSession(username string) (*protocol.HotSession, bool, error) {
	if username == "" {
		return nil, false, nil
	}
	var session protocol.HotSession
	ok, err := s.client.GetJSON(context.Background(), s.sessionKey(username), &session)
	if err != nil || !ok {
		return nil, ok, err
	}
	return &session, true, nil
}

func (s *RedisStore) DeleteHotSession(username string) error {
	return s.client.Del(context.Background(), s.sessionKey(username))
}

func (s *RedisStore) userKey(username string) string {
	return s.prefix + ":user:" + username
}

func (s *RedisStore) sessionKey(username string) string {
	return s.prefix + ":session:" + username
}

func marshalJSON(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
