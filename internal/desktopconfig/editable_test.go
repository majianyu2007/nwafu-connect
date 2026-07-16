package desktopconfig

import (
	"testing"

	"github.com/majianyu2007/nwafu-connect/configs"
)

func TestApplyEnforcesManagedBrowserAndAutomaticDNS(t *testing.T) {
	oldPassword := "saved-secret"
	configuration := configs.ConfigTOML{Password: &oldPassword}
	preferences := Preferences{}
	editable := Editable{
		Server: "vpn.nwafu.edu.cn", Port: 443, Username: "student", AuthType: "auth/psw", LoginDomain: "LDAP",
		BrowserURL: "https://lib.nwafu.edu.cn/", LaunchAtLogin: true,
	}
	if err := Apply(&configuration, &preferences, editable); err != nil {
		t.Fatal(err)
	}
	if configuration.Password == nil || *configuration.Password != oldPassword {
		t.Fatal("blank password did not preserve the stored password")
	}
	if configuration.BrowserMode == nil || !*configuration.BrowserMode {
		t.Fatal("browser mode was not enforced")
	}
	if configuration.DisableRemoteDNS == nil || *configuration.DisableRemoteDNS {
		t.Fatal("remote DNS was not enforced")
	}
	if configuration.RemoteDNSServer == nil || *configuration.RemoteDNSServer != "auto" {
		t.Fatalf("remote DNS = %v, want auto", configuration.RemoteDNSServer)
	}
	if configuration.SecondaryDNSServer == nil || *configuration.SecondaryDNSServer != "" {
		t.Fatalf("secondary DNS = %v, want isolated only", configuration.SecondaryDNSServer)
	}
	if !preferences.LaunchAtLogin {
		t.Fatal("launch-at-login preference was not saved")
	}
}

func TestApplyRejectsIncompletePasswordAuthentication(t *testing.T) {
	configuration := configs.ConfigTOML{}
	preferences := Preferences{}
	err := Apply(&configuration, &preferences, Editable{Server: "vpn.nwafu.edu.cn", Port: 443, AuthType: "auth/psw", Username: "student"})
	if err == nil {
		t.Fatal("missing password was accepted")
	}
}
