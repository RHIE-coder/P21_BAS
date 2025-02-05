package server

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
	"bytes"

	"bserver/oauth2"
	"bserver/oauth2/errors"
	"bserver/oauth2/server/bcconector"

	"github.com/hyperledger/fabric-sdk-go/pkg/gateway"
)

// NewDefaultServer create a default authorization server
func NewDefaultServer(manager oauth2.Manager) *Server {
	return NewServer(NewConfig(), manager)
}

// NewServer create authorization server
func NewServer(cfg *Config, manager oauth2.Manager) *Server {
	srv := &Server{
		Config:  cfg,
		Manager: manager,
	}



	// default handler
	srv.ClientInfoHandler = ClientBasicHandler

	srv.UserAuthorizationHandler = func(w http.ResponseWriter, r *http.Request) (string, error) {
		return "", errors.ErrAccessDenied
	}

	srv.PasswordAuthorizationHandler = func(username, password string) (string, error) {
		return "", errors.ErrAccessDenied
	}

	// DY mod START
	// Generate BCConnector
	srv.contract = bcconnector.NewConnector()
	// bcconnector.ReadCodeInfo(srv.contract,"TestCID")
	// DY mod END


	return srv
}

// Server Provide authorization server
type Server struct {
	Config                       *Config
	Manager                      oauth2.Manager
	ClientInfoHandler            ClientInfoHandler
	ClientAuthorizedHandler      ClientAuthorizedHandler
	ClientScopeHandler           ClientScopeHandler
	UserAuthorizationHandler     UserAuthorizationHandler
	PasswordAuthorizationHandler PasswordAuthorizationHandler
	RefreshingValidationHandler  RefreshingValidationHandler
	RefreshingScopeHandler       RefreshingScopeHandler
	ResponseErrorHandler         ResponseErrorHandler
	InternalErrorHandler         InternalErrorHandler
	ExtensionFieldsHandler       ExtensionFieldsHandler
	AccessTokenExpHandler        AccessTokenExpHandler
	AuthorizeScopeHandler        AuthorizeScopeHandler
	contract				     *gateway.Contract
}

func (s *Server) redirectError(w http.ResponseWriter, req *AuthorizeRequest, err error) error {
	if req == nil {
		return err
	}
	data, _, _ := s.GetErrorData(err)
	return s.redirect(w, req, data)
}

func (s *Server) redirect(w http.ResponseWriter, req *AuthorizeRequest, data map[string]interface{}) error {
	uri, err := s.GetRedirectURI(req, data)
	if err != nil {
		return err
	}

	w.Header().Set("Location", uri)
	w.WriteHeader(302)
	return nil
}

func (s *Server) tokenError(w http.ResponseWriter, err error) error {
	data, statusCode, header := s.GetErrorData(err)
	return s.token(w, data, header, statusCode)
}

func (s *Server) token(w http.ResponseWriter, data map[string]interface{}, header http.Header, statusCode ...int) error {
	w.Header().Set("Content-Type", "application/json;charset=UTF-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")

	for key := range header {
		w.Header().Set(key, header.Get(key))
	}

	status := http.StatusOK
	if len(statusCode) > 0 && statusCode[0] > 0 {
		status = statusCode[0]
	}

	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(data)
}

// GetRedirectURI get redirect uri
func (s *Server) GetRedirectURI(req *AuthorizeRequest, data map[string]interface{}) (string, error) {
	u, err := url.Parse(req.RedirectURI)
	if err != nil {
		return "", err
	}

	q := u.Query()
	if req.State != "" {
		q.Set("state", req.State)
	}

	for k, v := range data {
		q.Set(k, fmt.Sprint(v))
	}

	switch req.ResponseType {
	case oauth2.Code:
		u.RawQuery = q.Encode()
	case oauth2.Token:
		u.RawQuery = ""
		fragment, err := url.QueryUnescape(q.Encode())
		if err != nil {
			return "", err
		}
		u.Fragment = fragment
	}

	return u.String(), nil
}

// CheckResponseType check allows response type
func (s *Server) CheckResponseType(rt oauth2.ResponseType) bool {
	for _, art := range s.Config.AllowedResponseTypes {
		if art == rt {
			return true
		}
	}
	return false
}

// CheckCodeChallengeMethod checks for allowed code challenge method
func (s *Server) CheckCodeChallengeMethod(ccm oauth2.CodeChallengeMethod) bool {
	for _, c := range s.Config.AllowedCodeChallengeMethods {
		if c == ccm {
			return true
		}
	}
	return false
}

// ValidationAuthorizeRequest the authorization request validation
func (s *Server) ValidationAuthorizeRequest(r *http.Request) (*AuthorizeRequest, error) {
	redirectURI := r.FormValue("redirect_uri")
	clientID := r.FormValue("client_id")
	if !(r.Method == "GET" || r.Method == "POST") ||
		clientID == "" {
		return nil, errors.ErrInvalidRequest
	}

	resType := oauth2.ResponseType(r.FormValue("response_type"))
	if resType.String() == "" {
		return nil, errors.ErrUnsupportedResponseType
	} else if allowed := s.CheckResponseType(resType); !allowed {
		return nil, errors.ErrUnauthorizedClient
	}

	cc := r.FormValue("code_challenge")
	if cc == "" && s.Config.ForcePKCE {
		return nil, errors.ErrCodeChallengeRquired
	}
	if cc != "" && (len(cc) < 43 || len(cc) > 128) {
		return nil, errors.ErrInvalidCodeChallengeLen
	}

	ccm := oauth2.CodeChallengeMethod(r.FormValue("code_challenge_method"))
	// set default
	if ccm == "" {
		ccm = oauth2.CodeChallengePlain
	}
	if ccm.String() != "" && !s.CheckCodeChallengeMethod(ccm) {
		return nil, errors.ErrUnsupportedCodeChallengeMethod
	}

	req := &AuthorizeRequest{
		RedirectURI:         redirectURI,
		ResponseType:        resType,
		ClientID:            clientID,
		State:               r.FormValue("state"),
		Scope:               r.FormValue("scope"),
		Request:             r,
		CodeChallenge:       cc,
		CodeChallengeMethod: ccm,
	}
	return req, nil
}

// GetAuthorizeToken get authorization token(code)
func (s *Server) GetAuthorizeToken(ctx context.Context, req *AuthorizeRequest) (oauth2.TokenInfo, error) {
	// check the client allows the grant type
	if fn := s.ClientAuthorizedHandler; fn != nil {
		gt := oauth2.AuthorizationCode
		if req.ResponseType == oauth2.Token {
			gt = oauth2.Implicit
		}

		allowed, err := fn(req.ClientID, gt)
		if err != nil {
			return nil, err
		} else if !allowed {
			return nil, errors.ErrUnauthorizedClient
		}
	}

	// check the client allows the authorized scope
	if fn := s.ClientScopeHandler; fn != nil {
		tgr := &oauth2.TokenGenerateRequest{
			ClientID:       req.ClientID,
			UserID:         req.UserID,
			RedirectURI:    req.RedirectURI,
			Scope:          req.Scope,
			AccessTokenExp: req.AccessTokenExp,
			Request:        req.Request,
		}
		allowed, err := fn(tgr)
		if err != nil {
			return nil, err
		} else if !allowed {
			return nil, errors.ErrInvalidScope
		}
	}

	tgr := &oauth2.TokenGenerateRequest{
		ClientID:            req.ClientID,
		UserID:              req.UserID,
		RedirectURI:         req.RedirectURI,
		Scope:               req.Scope,
		AccessTokenExp:      req.AccessTokenExp,
		Request:             req.Request,
		CodeChallenge:       req.CodeChallenge,
		CodeChallengeMethod: req.CodeChallengeMethod,
	}
	return s.Manager.GenerateAuthToken(ctx, req.ResponseType, tgr)
}

// GetAuthorizeData get authorization response data
func (s *Server) GetAuthorizeData(rt oauth2.ResponseType, ti oauth2.TokenInfo) map[string]interface{} {
	if rt == oauth2.Code {
		return map[string]interface{}{
			"code": ti.GetCode(),
		}
	}
	return s.GetTokenData(ti)
}

// HandleAuthorizeRequest the authorization request handling
func (s *Server) HandleAuthorizeRequest(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()

	req, err := s.ValidationAuthorizeRequest(r)
	if err != nil {
		return s.redirectError(w, req, err)
	}

	// DY mod START
	// user authorization (->>> will change DID Auth)
	// userID, err := s.UserAuthorizationHandler(w, r)
	// if err != nil {
	// 	return s.redirectError(w, req, err)
	// } else if userID == "" {
	// 	return nil
	// }
	// req.UserID = userID

	req.UserID = "UserDID"
	// DY mod END

	// specify the scope of authorization
	if fn := s.AuthorizeScopeHandler; fn != nil {
		scope, err := fn(w, r)
		if err != nil {
			return err
		} else if scope != "" {
			req.Scope = scope
		}
	}

	// specify the expiration time of access token
	if fn := s.AccessTokenExpHandler; fn != nil {
		exp, err := fn(w, r)
		if err != nil {
			return err
		}
		req.AccessTokenExp = exp
	}

	ti, err := s.GetAuthorizeToken(ctx, req)
	if err != nil {
		return s.redirectError(w, req, err)
	}

	// If the redirect URI is empty, the default domain provided by the client is used.
	if req.RedirectURI == "" {
		client, err := s.Manager.GetClient(ctx, req.ClientID)
		if err != nil {
			return err
		}
		req.RedirectURI = client.GetDomain()
	}

	// DY mod START
	// Post code issuance (save code in blockchain)

	var mAuthorizeData = s.GetAuthorizeData(req.ResponseType, ti)
	var mCode = mAuthorizeData["code"].(string)

	log.Printf("-------------mAuthorizeData------------")
	log.Printf("Id_code : not yet\n")
	log.Printf("DID_RO : %s\n", req.UserID)
	log.Printf("DID_Client : %s\n", req.ClientID)
	log.Printf("Scope : %s\n", req.Scope)
	log.Printf("Hash_code : %s\n", genHashS256(mCode))
	log.Printf("Time_issued : not yet\n")
	log.Printf("URI_Redirection : %s\n", req.RedirectURI)
	log.Printf("Condition : Available\n")
	log.Printf("Id_token : not yet\n")
	log.Printf("%s\n", mAuthorizeData["code"])
	log.Printf("%s\n", mCode)
	log.Printf("Hash_code : %s\n", genHashS256(mCode))
	log.Printf("-------------mAuthorizeData------------")

	// CreateCodeInfo Test
	// mHashV := [32]byte{0}
	mHashV := genHashS256(mCode)	
	var mID bytes.Buffer
	mID.WriteString("CI_")
	mID.WriteString(mHashV)
	mCodeInfo := bcconnector.CodeInfo{
		InfoType:        "CodeInfo",
		ID_code:         mID.String(),
		DID_RO:          req.UserID,
		DID_client:      req.ClientID,
		Scope:           req.Scope,
		Hash_code:       mHashV,
		Time_issueed:    time.Now().String(),
		URI_Redirection: req.RedirectURI,
		Condition:       "Available",
		ID_token:        "",
	}
	// log.Printf("%s\n", mCodeInfo)	
	go bcconnector.CreateCodeInfo(s.contract, mCodeInfo)

	// Record the CodeInfo In BAS

	return s.redirect(w, req, mAuthorizeData)

	// DY mod END

	//return s.redirect(w, req, s.GetAuthorizeData(req.ResponseType, ti))

}

// DY mod START
// generate Hash value
func genHashS256(s string) string {
	s256 := sha256.Sum256([]byte(s))
	return base64.StdEncoding.EncodeToString(s256[:])
	//base64.StdEncoding.DecodeString(s256[:])
}

// DY mod END

// ValidationTokenRequest the token request validation
func (s *Server) ValidationTokenRequest(r *http.Request) (oauth2.GrantType, *oauth2.TokenGenerateRequest, error) {
	if v := r.Method; !(v == "POST" ||
		(s.Config.AllowGetAccessRequest && v == "GET")) {
		return "", nil, errors.ErrInvalidRequest
	}

	gt := oauth2.GrantType(r.FormValue("grant_type"))
	if gt.String() == "" {
		return "", nil, errors.ErrUnsupportedGrantType
	}

	clientID, clientSecret, err := s.ClientInfoHandler(r)
	if err != nil {
		return "", nil, err
	}

	tgr := &oauth2.TokenGenerateRequest{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Request:      r,
	}

	switch gt {
	case oauth2.AuthorizationCode:
		tgr.RedirectURI = r.FormValue("redirect_uri")
		tgr.Code = r.FormValue("code")
		if tgr.RedirectURI == "" ||
			tgr.Code == "" {
			return "", nil, errors.ErrInvalidRequest
		}
		tgr.CodeVerifier = r.FormValue("code_verifier")
		if s.Config.ForcePKCE && tgr.CodeVerifier == "" {
			return "", nil, errors.ErrInvalidRequest
		}
	case oauth2.PasswordCredentials:
		tgr.Scope = r.FormValue("scope")
		username, password := r.FormValue("username"), r.FormValue("password")
		if username == "" || password == "" {
			return "", nil, errors.ErrInvalidRequest
		}

		userID, err := s.PasswordAuthorizationHandler(username, password)
		if err != nil {
			return "", nil, err
		} else if userID == "" {
			return "", nil, errors.ErrInvalidGrant
		}
		tgr.UserID = userID
	case oauth2.ClientCredentials:
		tgr.Scope = r.FormValue("scope")
	case oauth2.Refreshing:
		tgr.Refresh = r.FormValue("refresh_token")
		tgr.Scope = r.FormValue("scope")
		if tgr.Refresh == "" {
			return "", nil, errors.ErrInvalidRequest
		}
	}
	return gt, tgr, nil
}

// CheckGrantType check allows grant type
func (s *Server) CheckGrantType(gt oauth2.GrantType) bool {
	for _, agt := range s.Config.AllowedGrantTypes {
		if agt == gt {
			return true
		}
	}
	return false
}

// GetAccessToken access token
func (s *Server) GetAccessToken(ctx context.Context, gt oauth2.GrantType, tgr *oauth2.TokenGenerateRequest) (oauth2.TokenInfo, error) {
	if allowed := s.CheckGrantType(gt); !allowed {
		return nil, errors.ErrUnauthorizedClient
	}

	if fn := s.ClientAuthorizedHandler; fn != nil {
		allowed, err := fn(tgr.ClientID, gt)
		if err != nil {
			return nil, err
		} else if !allowed {
			return nil, errors.ErrUnauthorizedClient
		}
	}

	switch gt {
	case oauth2.AuthorizationCode:
		ti, err := s.Manager.GenerateAccessToken(ctx, gt, tgr)
		if err != nil {
			switch err {
			case errors.ErrInvalidAuthorizeCode, errors.ErrInvalidCodeChallenge,
				errors.ErrMissingCodeChallenge, errors.ErrMissingCodeChallenge:
				return nil, errors.ErrInvalidGrant
			case errors.ErrInvalidClient:
				return nil, errors.ErrInvalidClient
			default:
				return nil, err
			}
		}
		return ti, nil
	case oauth2.PasswordCredentials, oauth2.ClientCredentials:
		if fn := s.ClientScopeHandler; fn != nil {
			allowed, err := fn(tgr)
			if err != nil {
				return nil, err
			} else if !allowed {
				return nil, errors.ErrInvalidScope
			}
		}
		return s.Manager.GenerateAccessToken(ctx, gt, tgr)
	case oauth2.Refreshing:
		// check scope
		if scope, scopeFn := tgr.Scope, s.RefreshingScopeHandler; scope != "" && scopeFn != nil {
			rti, err := s.Manager.LoadRefreshToken(ctx, tgr.Refresh)
			if err != nil {
				if err == errors.ErrInvalidRefreshToken || err == errors.ErrExpiredRefreshToken {
					return nil, errors.ErrInvalidGrant
				}
				return nil, err
			}

			allowed, err := scopeFn(tgr, rti.GetScope())
			if err != nil {
				return nil, err
			} else if !allowed {
				return nil, errors.ErrInvalidScope
			}
		}

		if validationFn := s.RefreshingValidationHandler; validationFn != nil {
			rti, err := s.Manager.LoadRefreshToken(ctx, tgr.Refresh)
			if err != nil {
				if err == errors.ErrInvalidRefreshToken || err == errors.ErrExpiredRefreshToken {
					return nil, errors.ErrInvalidGrant
				}
				return nil, err
			}
			allowed, err := validationFn(rti)
			if err != nil {
				return nil, err
			} else if !allowed {
				return nil, errors.ErrInvalidScope
			}
		}

		ti, err := s.Manager.RefreshAccessToken(ctx, tgr)
		if err != nil {
			if err == errors.ErrInvalidRefreshToken || err == errors.ErrExpiredRefreshToken {
				return nil, errors.ErrInvalidGrant
			}
			return nil, err
		}
		return ti, nil
	}

	return nil, errors.ErrUnsupportedGrantType
}

// GetTokenData token data
func (s *Server) GetTokenData(ti oauth2.TokenInfo) map[string]interface{} {
	data := map[string]interface{}{
		"access_token": ti.GetAccess(),
		"token_type":   s.Config.TokenType,
		"expires_in":   int64(ti.GetAccessExpiresIn() / time.Second),
	}

	if scope := ti.GetScope(); scope != "" {
		data["scope"] = scope
	}

	if refresh := ti.GetRefresh(); refresh != "" {
		data["refresh_token"] = refresh
	}

	if fn := s.ExtensionFieldsHandler; fn != nil {
		ext := fn(ti)
		for k, v := range ext {
			if _, ok := data[k]; ok {
				continue
			}
			data[k] = v
		}
	}
	return data
}

// HandleTokenRequest token request handling
func (s *Server) HandleTokenRequest(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()

	gt, tgr, err := s.ValidationTokenRequest(r)
	if err != nil {
		return s.tokenError(w, err)
	}

	// DY mod START
	// Authorization code validation
	log.Printf("-------------Authorization code validation------------")
	log.Printf("DID_Client : %s\n", tgr.ClientID)
	log.Printf("RedirectURI : %s\n", tgr.RedirectURI)
	log.Printf("code : %s\n", tgr.Code)
	log.Printf("-------------Authorization code validation------------")

	// DY mod END

	ti, err := s.GetAccessToken(ctx, gt, tgr)
	if err != nil {
		return s.tokenError(w, err)
	}

	// DY mod START	
	// Retrieve CodeInfo = mCodeInfo

	mCHashV := genHashS256(tgr.Code)
	var mCID bytes.Buffer
	mCID.WriteString("CI_")
	mCID.WriteString(mCHashV)
	var mCodeInfo bcconnector.CodeInfo
	mCodeInfo = bcconnector.ReadCodeInfo(s.contract,mCID.String())
	log.Printf("-------------CodeInfo------------")
	log.Printf("Id_Code :%s\n", mCodeInfo.ID_code)


	var mToken = ti.GetAccess()
	log.Printf("-------------TokenInfo------------")
	log.Printf("Id_Token : not yet\n")
	log.Printf("DID_RO (from CodeInfo)\n")
	log.Printf("DID_Client (from CodeInfo)")
	log.Printf("Scope (from CodeInfo)")
	log.Printf("Hash_token : %s\n", genHashS256(ti.GetAccess()))
	log.Printf("Time_issued : %s\n", ti.GetAccessCreateAt())
	log.Printf("Time_expiration : %s \n", ti.GetAccessExpiresIn())
	log.Printf("URI_Redirection : %s\n", tgr.RedirectURI)
	log.Printf("Condition : Available\n")
	log.Printf("CodeInfo - Id_token = Id_token : not yet\n")
	log.Printf("%s\n", mToken)
	log.Printf("Hash_code : %s\n", genHashS256(mToken))
	log.Printf("-------------mAuthorizeData------------")


	// CreateTokenInfo Test
	mTHashV := genHashS256(mToken)
	var mTID bytes.Buffer
	mTID.WriteString("TI_")
	mTID.WriteString(mTHashV)

	mTokenInfo := bcconnector.TokenInfo{
		InfoType:        "TokenInfo",
		ID_token:        mTID.String(),
		DID_RO:          mCodeInfo.DID_RO,
		DID_client:      mCodeInfo.DID_client,
		Scope:           mCodeInfo.Scope,
		Hash_token:      mTHashV,
		Time_issueed:    ti.GetAccessCreateAt().String(),
		Time_expiration: ti.GetAccessCreateAt().String(),
		URI_Redirection: tgr.RedirectURI,
		Condition:       "Available",
	}

	// Record the TokenInfo In BAS ( goroutine )
	go bcconnector.CreateTokenInfo(s.contract, mTokenInfo)

	// Record the TokenInfo In BAS (without goroutine)
	// bcconnector.CreateTokenInfo(s.contract, mTokenInfo)
	
	// Record the CodeInfo In BAS	
	
	// DY mod END

	return s.token(w, s.GetTokenData(ti), nil)

	// return err
}

// GetErrorData get error response data
func (s *Server) GetErrorData(err error) (map[string]interface{}, int, http.Header) {
	var re errors.Response
	if v, ok := errors.Descriptions[err]; ok {
		re.Error = err
		re.Description = v
		re.StatusCode = errors.StatusCodes[err]
	} else {
		if fn := s.InternalErrorHandler; fn != nil {
			if v := fn(err); v != nil {
				re = *v
			}
		}

		if re.Error == nil {
			re.Error = errors.ErrServerError
			re.Description = errors.Descriptions[errors.ErrServerError]
			re.StatusCode = errors.StatusCodes[errors.ErrServerError]
		}
	}

	if fn := s.ResponseErrorHandler; fn != nil {
		fn(&re)
	}

	data := make(map[string]interface{})
	if err := re.Error; err != nil {
		data["error"] = err.Error()
	}

	if v := re.ErrorCode; v != 0 {
		data["error_code"] = v
	}

	if v := re.Description; v != "" {
		data["error_description"] = v
	}

	if v := re.URI; v != "" {
		data["error_uri"] = v
	}

	statusCode := http.StatusInternalServerError
	if v := re.StatusCode; v > 0 {
		statusCode = v
	}

	return data, statusCode, re.Header
}

// BearerAuth parse bearer token
func (s *Server) BearerAuth(r *http.Request) (string, bool) {
	auth := r.Header.Get("Authorization")
	prefix := "Bearer "
	token := ""

	if auth != "" && strings.HasPrefix(auth, prefix) {
		token = auth[len(prefix):]
	} else {
		token = r.FormValue("access_token")
	}

	return token, token != ""
}

// ValidationBearerToken validation the bearer tokens
// https://tools.ietf.org/html/rfc6750
func (s *Server) ValidationBearerToken(r *http.Request) error {
	// ctx := r.Context()

	accessToken, ok := s.BearerAuth(r)
	if !ok {
		return errors.ErrInvalidAccessToken
	}

	// DY mod START
	// Make the LoadAccessTokeninBAS
	// LoadAccessToken  -> Retrieve TokenInfo = mCodeInfo
	return s.LoadAccessTokeninBAS(accessToken)
	// DY mod END
	
	// return s.Manager.LoadAccessToken(ctx, accessToken)
}

func (s *Server) LoadAccessTokeninBAS(accessToken string) error {

	log.Printf("-------------LoadAccessTokeninBAS------------")
	mTHashV := genHashS256(accessToken)
	var mTID bytes.Buffer
	mTID.WriteString("TI_")
	mTID.WriteString(mTHashV)
	var mTokenInfo bcconnector.TokenInfo
	log.Printf("Id_Token :%s\n", mTID.String())
	mTokenInfo = bcconnector.ReadTokenInfo(s.contract,mTID.String())
	
	log.Printf("Id_Token :%s\n", mTokenInfo.ID_token)

	if mTokenInfo.Hash_token != mTHashV{
		return  errors.ErrInvalidAccessToken
	}
	
	return nil
}
