package session

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Store 负责会话记录的本地安全保存和读取。
type Store struct {
	directory string
}

// NewDefaultStore 创建默认的本地会话存储。
//
// 可通过 OPSPILOT_SESSION_DIR 覆盖默认目录，便于测试和部署环境显式指定保存位置。
func NewDefaultStore() (*Store, error) {
	if directory := strings.TrimSpace(os.Getenv("OPSPILOT_SESSION_DIR")); directory != "" {
		return NewStore(directory), nil
	}

	configDirectory, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("获取用户配置目录失败: %w", err)
	}
	return NewStore(filepath.Join(configDirectory, "opspilot", "sessions")), nil
}

// NewID 创建一个适合作为会话文件名的唯一 ID。
func NewID() (string, error) {
	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", fmt.Errorf("生成会话 ID 失败: %w", err)
	}
	return fmt.Sprintf("session-%s-%x", time.Now().UTC().Format("20060102T150405.000000000Z"), suffix), nil
}

// NewStore 创建一个使用指定目录的会话存储。
func NewStore(directory string) *Store {
	return &Store{directory: directory}
}

// Save 以受限权限原子保存一份会话记录。
//
// 先写入同目录临时文件并同步到磁盘，再通过 Rename 替换目标文件，避免留下半截 JSON。
func (s *Store) Save(record Record) error {
	if err := validateRecord(record); err != nil {
		return err
	}
	if err := os.MkdirAll(s.directory, 0o700); err != nil {
		return fmt.Errorf("创建会话目录失败: %w", err)
	}
	if err := os.Chmod(s.directory, 0o700); err != nil {
		return fmt.Errorf("设置会话目录权限失败: %w", err)
	}

	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("编码会话记录失败: %w", err)
	}
	data = append(data, '\n')

	temporary, err := os.CreateTemp(s.directory, ".session-*.tmp")
	if err != nil {
		return fmt.Errorf("创建会话临时文件失败: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)

	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("设置会话文件权限失败: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("写入会话记录失败: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("同步会话记录失败: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("关闭会话临时文件失败: %w", err)
	}

	target := filepath.Join(s.directory, record.ID+".json")
	if err := os.Rename(temporaryName, target); err != nil {
		return fmt.Errorf("替换会话记录失败: %w", err)
	}
	if err := os.Chmod(target, 0o600); err != nil {
		return fmt.Errorf("设置会话文件权限失败: %w", err)
	}
	return nil
}

// Load 读取指定 ID 的会话记录。
func (s *Store) Load(id string) (Record, error) {
	if err := validateSessionID(id); err != nil {
		return Record{}, err
	}

	data, err := os.ReadFile(filepath.Join(s.directory, id+".json"))
	if err != nil {
		return Record{}, fmt.Errorf("读取会话记录失败: %w", err)
	}

	var record Record
	if err := json.Unmarshal(data, &record); err != nil {
		return Record{}, fmt.Errorf("解析会话记录失败: %w", err)
	}
	if err := validateRecord(record); err != nil {
		return Record{}, fmt.Errorf("会话记录无效: %w", err)
	}
	return record, nil
}

func validateRecord(record Record) error {
	if err := validateSessionID(record.ID); err != nil {
		return err
	}
	if record.Version <= 0 {
		return errors.New("会话记录版本无效")
	}
	return nil
}

func validateSessionID(id string) error {
	if strings.TrimSpace(id) == "" || id == "." || id == ".." {
		return errors.New("会话 ID 不能为空或无效")
	}
	if strings.ContainsAny(id, `/\\`) {
		return errors.New("会话 ID 不能包含路径分隔符")
	}
	return nil
}
