package auth

import (
 "encoding/json"
 "strings"
 "time"

 "github.com/majianyu2007/nwafu-connect/client/authchallenge"
)

// Preserve the campus desktop prompts behind upstream's challenge interface.
type campusChallengeHandler struct { *authchallenge.CLIHandler }

func newCampusChallengeHandler() authchallenge.Handler {
 return &campusChallengeHandler{authchallenge.NewCLIHandler(authchallenge.CLIOptions{})}
}

func (h *campusChallengeHandler) HandleCodeChallenge(c authchallenge.CodeChallenge) (authchallenge.CodeResponse, error) {
 title := "输入短信验证码"
 description := "请输入学校网关发送到已登记手机的验证码。"
 if c.Kind != authchallenge.CodeSMS { title = "输入动态验证码"; description = "请输入身份验证器当前显示的一次性验证码。" }
 code, err := readVerificationCode(title, description, c.CanSkipSecondaryAuth)
 response := authchallenge.CodeResponse{Code: code}
 if c.CanSkipSecondaryAuth && strings.HasPrefix(code, "$") {
  response.Code = strings.TrimPrefix(code, "$")
  response.SkipSecondaryAuth = true
 }
 return response, err
}

func (h *campusChallengeHandler) HandleClickCaptcha(c authchallenge.ClickCaptchaChallenge) (authchallenge.ClickCaptchaResponse, error) {
 if c.OutputPath != "" { return h.CLIHandler.HandleClickCaptcha(c) }
 raw, err := serveCaptchaInBrowser(c.Image, 5*time.Minute)
 if err != nil { return authchallenge.ClickCaptchaResponse{}, err }
 var payload graphCheckCodePayload
 if err := json.Unmarshal([]byte(raw), &payload); err != nil { return authchallenge.ClickCaptchaResponse{}, err }
 response := authchallenge.ClickCaptchaResponse{Width: payload.Width, Height: payload.Height}
 for _, point := range payload.Coordinates {
  if len(point) == 2 { response.Points = append(response.Points, authchallenge.Point{X: point[0], Y: point[1]}) }
 }
 return response, nil
}
