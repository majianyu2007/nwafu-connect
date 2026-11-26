package auth

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/majianyu2007/nwafu-connect/client/authchallenge"
	"github.com/majianyu2007/nwafu-connect/log"
)

type HTTPSOauth2Login struct {
	Domain string
	Code   string
}

func (m HTTPSOauth2Login) AuthType() string {
	return "auth/httpsOauth2"
}

func (m HTTPSOauth2Login) LoginDomain() string {
	return m.Domain
}

func (m HTTPSOauth2Login) login(s *Session, authInfo AuthInfo) error {
	return s.loginAuthHTTPSOauth2(authInfo, m.Domain, m.Code)
}

func (s *Session) loginAuthHTTPSOauth2(authInfo AuthInfo, loginDomain, oauth2Code string) error {
	loginURL, err := url.Parse(authInfo.LoginURL)
	if err != nil {
		return err
	}

	var callback string
	if oauth2Code == "" {
		callback, err = s.interactiveHttpsOauth2(loginURL.String())
		if err != nil {
			return err
		}
	} else {
		callback = s.baseURL + "/passport/v1/auth/httpsOauth2?sfDomain=" + url.QueryEscape(loginDomain) + "&code=" + url.QueryEscape(oauth2Code)
	}

	if err := s.validateHTTPSOauth2CallbackURL(callback, loginDomain); err != nil {
		return err
	}

	if err := s.httpsOauth2(callback); err != nil {
		return err
	}
	if _, _, err := s.authConfig(true, false); err != nil {
		return err
	}
	return nil
}

func (s *Session) interactiveHttpsOauth2(loginURL string) (string, error) {
	response, err := s.handler().HandleExternalLogin(authchallenge.ExternalLoginChallenge{
		Kind:     authchallenge.ExternalLoginOAuth2,
		LoginURL: loginURL,
		Message:  "Input callback url:",
	})
	if err != nil {
		return "", fmt.Errorf("complete HTTPS OAuth2 login challenge: %w", err)
	}
	return response.CallbackURL, nil
}

func (s *Session) validateHTTPSOauth2CallbackURL(callback, loginDomain string) error {
	parsedURL, err := url.Parse(callback)
	if err != nil {
		return fmt.Errorf("invalid HTTPS OAuth2 callback url: %w", err)
	}
	if parsedURL.Scheme != "https" || !sameHTTPSAuthority(parsedURL, s.baseHost) {
		return fmt.Errorf("invalid HTTPS OAuth2 callback url: host not match")
	}
	if parsedURL.Path != "/passport/v1/auth/httpsOauth2" {
		return fmt.Errorf("invalid HTTPS OAuth2 callback url: path not match")
	}
	queries := parsedURL.Query()
	if queries.Get("sfDomain") != loginDomain {
		return fmt.Errorf("invalid HTTPS OAuth2 callback url: login domain not match")
	}
	if strings.TrimSpace(queries.Get("code")) == "" {
		return fmt.Errorf("invalid HTTPS OAuth2 callback url: code not found")
	}
	return nil
}

func (s *Session) httpsOauth2(callback string) error {
	log.Println("Perform GET /passport/v1/auth/httpsOauth2")

	req, err := http.NewRequest("GET", callback, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("x-csrf-token", s.csrfToken)
	req.Header.Set("x-sdp-rid", s.rid)
	req.Header.Set("x-sdp-traceid", s.randSdpId())

	noRedirectClient := *s.client
	noRedirectClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	origClient := s.client
	s.client = &noRedirectClient
	resp, err := s.request(req)
	s.client = origClient
	if err != nil {
		return err
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	if resp.StatusCode != http.StatusFound {
		return fmt.Errorf("invalid status code: %d", resp.StatusCode)
	}

	ticket, err := parsePortalTicketFromRedirect(resp.Header.Get("Location"), s.baseHost)
	if err != nil {
		return err
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	log.DebugPrintf("Received httpsOauth2 data: %s", string(body))

	s.ticket = ticket
	return nil
}
