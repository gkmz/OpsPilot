package config

import (
	"testing"

	opserrors "github.com/gkmz/opspilot/internal/errors"
)

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{name: "valid", config: Config{APIKey: "secret", BaseURL: "http://localhost:8080/v1", Model: "test", Timeout: 1}, wantErr: false},
		{name: "missing key", config: Config{BaseURL: "http://localhost:8080/v1", Model: "test", Timeout: 1}, wantErr: true},
		{name: "missing model", config: Config{APIKey: "secret", BaseURL: "http://localhost:8080/v1", Timeout: 1}, wantErr: true},
		{name: "invalid url", config: Config{APIKey: "secret", BaseURL: "not-url", Model: "test", Timeout: 1}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %t", err, tt.wantErr)
			}
			if tt.wantErr && opserrors.KindOf(err) != opserrors.KindConfig {
				t.Fatalf("Validate() kind = %q, want %q", opserrors.KindOf(err), opserrors.KindConfig)
			}
		})
	}
}

func TestLoadFromEnvRejectsInvalidTimeout(t *testing.T) {
	t.Setenv("OPSPILOT_TIMEOUT", "not-duration")
	if _, err := LoadFromEnv(); err == nil {
		t.Fatal("LoadFromEnv() expected invalid timeout error")
	} else if opserrors.KindOf(err) != opserrors.KindConfig {
		t.Fatalf("LoadFromEnv() kind = %q, want %q", opserrors.KindOf(err), opserrors.KindConfig)
	}
}
