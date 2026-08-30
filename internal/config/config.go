// Package config 负责加载和校验 OpsPilot 的运行配置。
package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	opserrors "github.com/gkmz/opspilot/internal/errors"
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
// Base URL、API Key 和模型名称必须由调用者显式配置，避免程序默认绑定特定模型服务商。
func LoadFromEnv() (Config, error) {
	timeout := defaultTimeout
	if raw := strings.TrimSpace(os.Getenv("OPSPILOT_TIMEOUT")); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil || parsed <= 0 {
			return Config{}, opserrors.Wrap(opserrors.KindConfig, fmt.Sprintf("OPSPILOT_TIMEOUT 无效: %q", raw), err)
		}
		timeout = parsed
	}

	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("OPSPILOT_BASE_URL")), "/")
	if baseURL == "" {
		return Config{}, opserrors.Wrap(opserrors.KindConfig, "未设置 OPSPILOT_BASE_URL", nil)
	}

	apiKey := strings.TrimSpace(os.Getenv("OPSPILOT_API_KEY"))
	if apiKey == "" {
		return Config{}, opserrors.Wrap(opserrors.KindConfig, "OPSPILOT_API_KEY 未设置", nil)
	}

	modelName := strings.TrimSpace(os.Getenv("OPSPILOT_MODEL"))
	if modelName == "" {
		return Config{}, opserrors.Wrap(opserrors.KindConfig, "未设置 OPSPILOT_MODEL", nil)
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
		return opserrors.Wrap(opserrors.KindConfig, "缺少 OPSPILOT_BASE_URL", nil)
	}
	if c.APIKey == "" {
		return opserrors.Wrap(opserrors.KindConfig, "缺少 OPSPILOT_API_KEY", nil)
	}
	if c.Model == "" {
		return opserrors.Wrap(opserrors.KindConfig, "缺少 OPSPILOT_MODEL", nil)
	}
	parsed, err := url.ParseRequestURI(c.BaseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return opserrors.Wrap(opserrors.KindConfig, fmt.Sprintf("OPSPILOT_BASE_URL 无效: %q", c.BaseURL), err)
	}
	if c.Timeout <= 0 {
		return opserrors.Wrap(opserrors.KindConfig, "OPSPILOT_TIMEOUT 必须大于 0", nil)
	}
	return nil
}
