package desktop

import (
	"testing"
	"time"

	"github.com/majianyu2007/nwafu-connect/configs"
)

func TestConfiguredRequiresCredentialsForSelectedAuthentication(t *testing.T) {
	server, passwordAuth, smsAuth, wechatAuth := "vpn.nwafu.edu.cn", "auth/psw", "auth/smsCheckCode", "auth/qywechat"
	username, password, phone := "student", "secret", "86-13800138000"
	tests := []struct {
		name          string
		configuration configs.ConfigTOML
		want          bool
	}{
		{name: "missing password", configuration: configs.ConfigTOML{ServerAddress: &server, AuthType: &passwordAuth, Username: &username}},
		{name: "password ready", configuration: configs.ConfigTOML{ServerAddress: &server, AuthType: &passwordAuth, Username: &username, Password: &password}, want: true},
		{name: "SMS ready", configuration: configs.ConfigTOML{ServerAddress: &server, AuthType: &smsAuth, Phone: &phone}, want: true},
		{name: "WeCom opens interactive login", configuration: configs.ConfigTOML{ServerAddress: &server, AuthType: &wechatAuth}, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := configured(test.configuration); got != test.want {
				t.Fatalf("configured() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestShouldReconnectAfterWakeOrNetworkRecovery(t *testing.T) {
	tests := []struct {
		name                 string
		elapsed              time.Duration
		wasOnline, nowOnline bool
		want                 bool
	}{
		{name: "normal online tick", elapsed: 15 * time.Second, wasOnline: true, nowOnline: true},
		{name: "wake from sleep", elapsed: 2 * time.Minute, wasOnline: true, nowOnline: true, want: true},
		{name: "network recovered", elapsed: 15 * time.Second, wasOnline: false, nowOnline: true, want: true},
		{name: "still offline", elapsed: 15 * time.Second, wasOnline: false, nowOnline: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := shouldReconnect(test.elapsed, test.wasOnline, test.nowOnline); got != test.want {
				t.Fatalf("shouldReconnect() = %t, want %t", got, test.want)
			}
		})
	}
}
