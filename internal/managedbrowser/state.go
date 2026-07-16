package managedbrowser

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type State struct {
	ProxyAddress string `json:"proxy_address"`
	StartURL     string `json:"start_url"`
	Executable   string `json:"executable"`
	ProfileDir   string `json:"profile_dir"`
}

func WriteState(path string, state State) error {
	if path == "" {
		return errors.New("browser state path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create browser state directory: %w", err)
	}
	payload, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode browser state: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".browser-state-*")
	if err != nil {
		return fmt.Errorf("create browser state file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return fmt.Errorf("protect browser state file: %w", err)
	}
	if _, err := temporary.Write(payload); err != nil {
		temporary.Close()
		return fmt.Errorf("write browser state file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close browser state file: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("publish browser state file: %w", err)
	}
	return nil
}

func ReadState(path string) (State, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return State{}, fmt.Errorf("read browser state file: %w", err)
	}
	var state State
	if err := json.Unmarshal(payload, &state); err != nil {
		return State{}, fmt.Errorf("decode browser state file: %w", err)
	}
	if state.ProxyAddress == "" || state.StartURL == "" || state.Executable == "" {
		return State{}, errors.New("browser state is incomplete")
	}
	return state, nil
}

func RemoveState(path string) error {
	if path == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove browser state file: %w", err)
	}
	return nil
}
