package embed

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigDefaults(t *testing.T) {
	c := Config{}.withDefaults()
	if c.Server != DefaultServer || c.Port != 443 || c.HTTPBind != "127.0.0.1:0" ||
		c.LoginDomain != "LDAP" || c.RemoteDNSServer != "auto" || c.DNSTTL != 3600 ||
		c.UpdateBestNodesInterval != 300 {
		t.Fatalf("defaults = %+v", c)
	}
}

func TestValidate(t *testing.T) {
	ok := Config{Username: "u", Password: "p"}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	bad := []Config{
		{Password: "p"},
		{Username: "u"},
		{Username: "u", Password: "p", Port: -1},
		{Username: "u", Password: "p", HTTPBind: "localhost"},
		{Username: "u", Password: "p", UpdateBestNodesInterval: -5},
	}
	for _, c := range bad {
		if err := c.Validate(); err == nil {
			t.Errorf("Validate(%+v) = nil", c)
		}
	}
}

func TestStartRejectsInvalidConfigWithoutNetwork(t *testing.T) {
	if _, err := Start(context.Background(), Config{}, nil); err == nil {
		t.Fatal("Start with empty config succeeded")
	}
}

func TestNilPrompterFailsInteractiveSteps(t *testing.T) {
	a := promptAdapter{ctx: context.Background(), p: errPrompter{}}
	if _, err := a.GraphCheckCode([]byte{1}); err == nil {
		t.Fatal("captcha without prompter should fail")
	}
	if _, err := a.VerificationCode("sms", "", "", false); err == nil || !strings.Contains(err.Error(), "sms") {
		t.Fatalf("sms err = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (promptAdapter{ctx: ctx, p: errPrompter{}}).GraphCheckCode(nil); err != context.Canceled {
		t.Fatalf("cancelled ctx err = %v", err)
	}
}

func TestWritePrivateFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "client.json")
	if err := writePrivateFile(path, []byte(`{"a":1}`)); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v", info.Mode().Perm())
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	s := &Session{state: StateRunning, proxyAddr: "127.0.0.1:1"}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if st := s.Status(); st.State != StateStopped || st.ProxyAddr != "" {
		t.Fatalf("status = %+v", st)
	}
}
