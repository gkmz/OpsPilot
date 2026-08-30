package cli

import (
	"bytes"
	"testing"
)

func TestNewRootCommandRegistersDiagnoseCommand(t *testing.T) {
	command := NewRootCommand(bytes.NewBuffer(nil), &bytes.Buffer{}, &bytes.Buffer{})

	diagnose, _, err := command.Find([]string{"diagnose"})
	if err != nil {
		t.Fatalf("Find(diagnose) error = %v", err)
	}
	if diagnose == nil || diagnose.Use != "diagnose [symptom]" {
		t.Fatalf("diagnose command = %+v", diagnose)
	}
}

func TestNewRootCommandConfiguresQuietUsage(t *testing.T) {
	command := NewRootCommand(bytes.NewBuffer(nil), &bytes.Buffer{}, &bytes.Buffer{})
	if !command.SilenceUsage {
		t.Fatal("root command should not print usage for runtime errors")
	}
}
