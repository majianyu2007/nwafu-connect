package desktopconfig

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/majianyu2007/nwafu-connect/configs"
)

type Editable struct {
	Server        string
	Port          int
	Username      string
	Password      string
	TOTPSecret    string
	AuthType      string
	Phone         string
	LoginDomain   string
	BrowserPath   string
	BrowserURL    string
	LaunchAtLogin bool
	HasPassword   bool
	HasTOTP       bool
}

func FromConfiguration(configuration configs.ConfigTOML, preferences Preferences) Editable {
	return Editable{
		Server: stringValue(configuration.ServerAddress, "vpn.nwafu.edu.cn"), Port: intValue(configuration.ServerPort, 443),
		Username: value(configuration.Username), AuthType: stringValue(configuration.AuthType, "auth/psw"), Phone: value(configuration.Phone),
		LoginDomain: stringValue(configuration.LoginDomain, "LDAP"), BrowserPath: value(configuration.BrowserPath), BrowserURL: value(configuration.BrowserURL),
		LaunchAtLogin: preferences.LaunchAtLogin, HasPassword: value(configuration.Password) != "", HasTOTP: value(configuration.TOTPSecret) != "",
	}
}

func Apply(configuration *configs.ConfigTOML, preferences *Preferences, editable Editable) error {
	server := strings.TrimSpace(editable.Server)
	if server == "" || strings.ContainsAny(server, "/: ") {
		return fmt.Errorf("服务器地址格式不正确")
	}
	if editable.Port < 1 || editable.Port > 65535 {
		return fmt.Errorf("服务器端口必须在 1–65535 之间")
	}
	if editable.AuthType != "auth/psw" && editable.AuthType != "auth/smsCheckCode" && editable.AuthType != "auth/qywechat" {
		return fmt.Errorf("不支持的认证方式")
	}
	username := strings.TrimSpace(editable.Username)
	phone := strings.TrimSpace(editable.Phone)
	if editable.AuthType == "auth/psw" && username == "" {
		return fmt.Errorf("密码认证需要填写用户名")
	}
	if editable.AuthType == "auth/psw" && value(configuration.Password) == "" && editable.Password == "" {
		return fmt.Errorf("密码认证需要填写密码")
	}
	if editable.AuthType == "auth/smsCheckCode" && phone == "" {
		return fmt.Errorf("短信认证需要填写手机号")
	}
	browserURL := strings.TrimSpace(editable.BrowserURL)
	if browserURL != "" {
		parsed, err := url.Parse(browserURL)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return fmt.Errorf("浏览器起始页必须是有效的 HTTP 或 HTTPS 地址")
		}
	}
	loginDomain := strings.TrimSpace(editable.LoginDomain)
	if loginDomain == "" {
		loginDomain = "LDAP"
	}
	browserPath := strings.TrimSpace(editable.BrowserPath)
	browserMode := true
	disableRemoteDNS := false
	remoteDNS := "auto"
	secondaryDNS := ""
	configuration.ServerAddress = &server
	configuration.ServerPort = &editable.Port
	configuration.Username = &username
	configuration.AuthType = &editable.AuthType
	configuration.Phone = &phone
	configuration.LoginDomain = &loginDomain
	configuration.BrowserPath = optionalPointer(browserPath)
	configuration.BrowserURL = &browserURL
	configuration.BrowserMode = &browserMode
	configuration.DisableRemoteDNS = &disableRemoteDNS
	configuration.RemoteDNSServer = &remoteDNS
	configuration.SecondaryDNSServer = &secondaryDNS
	if editable.Password != "" {
		configuration.Password = &editable.Password
	}
	if totp := strings.TrimSpace(editable.TOTPSecret); totp != "" {
		configuration.TOTPSecret = &totp
	}
	preferences.LaunchAtLogin = editable.LaunchAtLogin
	return nil
}

func value(pointer *string) string {
	if pointer == nil {
		return ""
	}
	return *pointer
}

func stringValue(pointer *string, fallback string) string {
	if pointer == nil || *pointer == "" {
		return fallback
	}
	return *pointer
}

func intValue(pointer *int, fallback int) int {
	if pointer == nil {
		return fallback
	}
	return *pointer
}

func optionalPointer(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
