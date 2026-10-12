package auth

import "testing"

type recordingPrompter struct{ kind string }

func (p *recordingPrompter) GraphCheckCode([]byte) (string, error) { return "[[1,2]]", nil }
func (p *recordingPrompter) VerificationCode(kind, _, _ string, _ bool) (string, error) {
	p.kind = kind
	return "  123456 ", nil
}

func TestReadVerificationCodeUsesPrompter(t *testing.T) {
	p := &recordingPrompter{}
	s := &Session{prompter: p}
	code, err := s.readVerificationCode("sms", "t", "h", true)
	if err != nil || code != "123456" || p.kind != "sms" {
		t.Fatalf("code=%q err=%v kind=%q", code, err, p.kind)
	}
}
