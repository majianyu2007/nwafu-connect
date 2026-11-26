package auth

import (
	"testing"

	"github.com/majianyu2007/nwafu-connect/client/authchallenge"
)

type recordingPrompter struct{ kind string }

func (p *recordingPrompter) GraphCheckCode([]byte) (string, error) { return "[[1,2]]", nil }
func (p *recordingPrompter) VerificationCode(kind, _, _ string, _ bool) (string, error) {
	p.kind = kind
	return "  123456 ", nil
}

func TestPrompterAdaptsToChallengeHandler(t *testing.T) {
	p := &recordingPrompter{}
	h := newPrompterChallengeHandler(p)
	resp, err := h.HandleCodeChallenge(authchallenge.CodeChallenge{
		Kind:                 authchallenge.CodeSMS,
		Message:              "h",
		CanSkipSecondaryAuth: true,
	})
	if err != nil || resp.Code != "123456" || p.kind != "sms" {
		t.Fatalf("resp=%+v err=%v kind=%q", resp, err, p.kind)
	}
	captcha, err := h.HandleClickCaptcha(authchallenge.ClickCaptchaChallenge{Image: []byte("img")})
	if err != nil || len(captcha.Points) != 1 || captcha.Points[0].X != 1 || captcha.Points[0].Y != 2 {
		t.Fatalf("captcha=%+v err=%v", captcha, err)
	}
}
