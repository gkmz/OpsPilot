// Package config 负责加载和校验 OpsPilot 的运行配置。
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	defaultTimeout = 60 * time.Second
)

// Config 保存单次模型调用所需的最小配置。
//
// 本阶段只抽象出可配置的端点，不引入完整 Provider 路由；路由和多模型能力属于后续模块。
type Config struct {
	APIKey  string
	BaseURL string
	Model   string
	Timeout time.Duration
}

// LoadFromEnv 从环境变量加载配置。
//
// 支持 OPSPILOT_API_KEY、OPSPILOT_BASE_URL、OPSPILOT_MODEL 和 OPSPILOT_TIMEOUT。
// Base URL 默认使用 DeepSeek 的 OpenAI 兼容接口，实际模型名称仍必须由调用者明确指定。
func LoadFromEnv() (Config, error) {
	timeout := defaultTimeout
	if raw := strings.TrimSpace(os.Getenv("OPSPILOT_TIMEOUT")); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil || parsed <= 0 {
			return Config{}, fmt.Errorf("OPSPILOT_TIMEOUT 无效: %q", raw)
		}
		timeout = parsed
	}

	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("OPSPILOT_BASE_URL")), "/")
	if baseURL == "" {
		return Config{}, fmt.Errorf("未设置 OPSPILOT_BASE_URL")
	}

	apiKey := strings.TrimSpace(os.Getenv("OPSPILOT_API_KEY"))
	if apiKey == "" {
		return Config{}, fmt.Errorf("OPSPILOT_API_KEY 未设置")
	}

	modelName := strings.TrimSpace(os.Getenv("OPSPILOT_MODEL"))
	if modelName == "" {
		return Config{}, fmt.Errorf("未设置 OPSPILOT_MODEL")
	}

	return Config{
		APIKey:  apiKey,
		BaseURL: baseURL,
		Model:   modelName,
		Timeout: timeout,
	}, nil
}

// Validate 检查配置是否满足单次模型调用的最低要求。
func (c Config) Validate() error {
	if c.BaseURL == "" {
		return errors.New("缺少 OPSPILOT_BASE_URL")
	}
	if c.APIKey == "" {
		return errors.New("缺少 OPSPILOT_API_KEY")
	}
	if c.Model == "" {
		return errors.New("缺少 OPSPILOT_MODEL")
	}
	parsed, err := url.ParseRequestURI(c.BaseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("OPSPILOT_BASE_URL 无效: %q", c.BaseURL)
	}
	if c.Timeout <= 0 {
		return errors.New("OPSPILOT_TIMEOUT 必须大于 0")
	}
	return nil
}
