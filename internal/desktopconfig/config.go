package desktopconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/majianyu2007/nwafu-connect/configs"
)

type Preferences struct {
	LaunchAtLogin bool `json:"launch_at_login"`
}

type Store struct {
	ConfigPath      string
	PreferencesPath string
	ClientDataPath  string
}

func (s Store) Load() (configs.ConfigTOML, Preferences, error) {
	configuration := s.defaults()
	if _, err := toml.DecodeFile(s.ConfigPath, &configuration); err != nil && !errors.Is(err, os.ErrNotExist) {
		return configs.ConfigTOML{}, Preferences{}, fmt.Errorf("read desktop configuration: %w", err)
	}
	preferences := Preferences{}
	payload, err := os.ReadFile(s.PreferencesPath)
	if err == nil {
		if err := json.Unmarshal(payload, &preferences); err != nil {
			return configs.ConfigTOML{}, Preferences{}, fmt.Errorf("read desktop preferences: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return configs.ConfigTOML{}, Preferences{}, fmt.Errorf("read desktop preferences: %w", err)
	}
	return configuration, preferences, nil
}

func (s Store) Save(configuration configs.ConfigTOML, preferences Preferences) error {
	var document bytes.Buffer
	if err := toml.NewEncoder(&document).Encode(configuration); err != nil {
		return fmt.Errorf("encode desktop configuration: %w", err)
	}
	if err := writePrivateFile(s.ConfigPath, document.Bytes()); err != nil {
		return err
	}
	payload, err := json.Marshal(preferences)
	if err != nil {
		return fmt.Errorf("encode desktop preferences: %w", err)
	}
	if err := writePrivateFile(s.PreferencesPath, payload); err != nil {
		return err
	}
	return nil
}

func (s Store) defaults() configs.ConfigTOML {
	server := "vpn.nwafu.edu.cn"
	port := 443
	authType := "auth/psw"
	loginDomain := "LDAP"
	browserMode := true
	disableRemoteDNS := false
	remoteDNS := "auto"
	secondaryDNS := ""
	clientData := s.ClientDataPath
	qrTerminal := false
	qrBrowser := true
	qrFile := filepath.Join(filepath.Dir(s.ConfigPath), "qywechat_qrcode.png")
	return configs.ConfigTOML{
		ServerAddress:          &server,
		ServerPort:             &port,
		AuthType:               &authType,
		LoginDomain:            &loginDomain,
		BrowserMode:            &browserMode,
		DisableRemoteDNS:       &disableRemoteDNS,
		RemoteDNSServer:        &remoteDNS,
		SecondaryDNSServer:     &secondaryDNS,
		ClientDataFile:         &clientData,
		QYWechatQRCodeTerminal: &qrTerminal,
		QYWechatQRCodeBrowser:  &qrBrowser,
		QYWechatQRCodeFile:     &qrFile,
	}
}

func writePrivateFile(path string, payload []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create private configuration directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".desktop-config-*")
	if err != nil {
		return fmt.Errorf("create private configuration file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return fmt.Errorf("protect private configuration file: %w", err)
	}
	if _, err := temporary.Write(payload); err != nil {
		temporary.Close()
		return fmt.Errorf("write private configuration file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close private configuration file: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("publish private configuration file: %w", err)
	}
	return nil
}
