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

type CASLogin struct {
	Domain string
	Ticket string
}

func (m CASLogin) AuthType() string {
	return "auth/cas"
}

func (m CASLogin) LoginDomain() string {
	return m.Domain
}

func (m CASLogin) login(s *Session, authInfo AuthInfo) error {
	return s.loginAuthCas(authInfo, m.Domain, m.Ticket)
}

func (s *Session) loginAuthCas(authInfo AuthInfo, loginDomain, casTicket string) error {
	loginURL, err := url.Parse(authInfo.LoginURL)
	if err != nil {
		return err
	}
	query := loginURL.Query()
	query.Set("service", s.baseURL+"/passport/v1/auth/cas?sfDomain="+loginDomain)
	loginURL.RawQuery = query.Encode()

	var callback string
	if casTicket == "" {
		callback, err = s.interactiveCas(loginURL.String())
		if err != nil {
			return err
		}
	} else {
		callback = s.baseURL + "/passport/v1/auth/cas?sfDomain=" + url.QueryEscape(loginDomain) + "&ticket=" + url.QueryEscape(casTicket)
	}

	if err := s.validateCASCallbackURL(callback, loginDomain); err != nil {
		return err
	}

	if err := s.cas(callback); err != nil {
		return err
	}
	if _, _, err := s.authConfig(true, false); err != nil {
		return err
	}
	return nil
}

func (s *Session) interactiveCas(loginURL string) (string, error) {
	response, err := s.handler().HandleExternalLogin(authchallenge.ExternalLoginChallenge{
		Kind:     authchallenge.ExternalLoginCAS,
		LoginURL: loginURL,
		Message:  "Input callback url:",
	})
	if err != nil {
		return "", fmt.Errorf("complete CAS login challenge: %w", err)
	}
	return response.CallbackURL, nil
}

func (s *Session) validateCASCallbackURL(callback, loginDomain string) error {
	parsedURL, err := url.Parse(callback)
	if err != nil {
		return fmt.Errorf("invalid CAS callback url: %w", err)
	}
	if parsedURL.Scheme != "https" || !sameHTTPSAuthority(parsedURL, s.baseHost) {
		return fmt.Errorf("invalid CAS callback url: host not match")
	}
	if parsedURL.Path != "/passport/v1/auth/cas" {
		return fmt.Errorf("invalid CAS callback url: path not match")
	}
	queries := parsedURL.Query()
	if queries.Get("sfDomain") != loginDomain {
		return fmt.Errorf("invalid CAS callback url: login domain not match")
	}
	if strings.TrimSpace(queries.Get("ticket")) == "" {
		return fmt.Errorf("invalid CAS callback url: ticket not found")
	}
	return nil
}

func (s *Session) cas(callback string) error {
	log.Println("Perform GET /passport/v1/auth/cas")

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
	log.DebugPrintf("Received cas data: %s", string(body))

	s.ticket = ticket
	return nil
}
