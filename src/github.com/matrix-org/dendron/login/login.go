package login

import (
	"bytes"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/matrix-org/dendron/proxy"

	"golang.org/x/crypto/bcrypt"
	"gopkg.in/macaroon.v1"
)

type matrixLoginRequest struct {
	Type     string `json:"type"`
	Password string `json:"password"`
	UserID   string `json:"user"`
}

type matrixLoginResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	HomeServer   string `json:"home_server"`
	UserID       string `json:"user_id"`
}

// A MatrixLoginHandler handles matrix login requests either using a database
// or by proxying the request to a synapse.
type MatrixLoginHandler struct {
	db             database
	proxy          *proxy.SynapseProxy
	serverName     string
	macaroonSecret string
}

// NewHandler makes a new MatrixLoginHandler.
func NewHandler(db *sql.DB, proxy *proxy.SynapseProxy, serverName, macaroonSecret string) (*MatrixLoginHandler, error) {
	sqlDB, err := makeSQLDatabase(db)
	if err != nil {
		return nil, err
	}

	return &MatrixLoginHandler{
		db:             sqlDB,
		proxy:          proxy,
		serverName:     serverName,
		macaroonSecret: macaroonSecret,
	}, err
}

func (h *MatrixLoginHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	defer req.Body.Close()
	if req.Method != "POST" {
		h.proxy.ProxyHTTP(w, req.Method, req.URL, req.Body, req.ContentLength, req.Header)
		return
	}

	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		proxy.LogAndReplyError(w, &proxy.HTTPError{err, 500, "M_UKNOWN", "Error reading request"})
	}

	var login matrixLoginRequest
	if err := json.Unmarshal(body, &login); err != nil {
		proxy.LogAndReplyError(w, &proxy.HTTPError{err, 400, "M_BAD_JSON", "Error decoding JSON"})
		return
	}

	switch login.Type {
	case "m.login.password":
		response, err := h.loginPassword(login.UserID, login.Password)
		if err != nil {
			proxy.LogAndReplyError(w, err)
			return
		}

		proxy.SetHeaders(w)

		json.NewEncoder(w).Encode(response)
	default:
		h.proxy.ProxyHTTP(w, req.Method, req.URL, bytes.NewBuffer(body), req.ContentLength, req.Header)
	}
}

func (h *MatrixLoginHandler) loginPassword(userID string, password string) (*matrixLoginResponse, *proxy.HTTPError) {

	if !strings.HasPrefix(userID, "@") {
		userID = "@" + userID + ":" + h.serverName
	}

	hash, err := h.db.passwordHash(userID)
	if err != nil {
		return nil, &proxy.HTTPError{err, 403, "M_FORBIDDEN", "Forbidden"}
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return nil, &proxy.HTTPError{err, 403, "M_FORBIDDEN", "Forbidden"}
	}

	expires := time.Now().Add(time.Hour)
	nonce, err := randomBase64(8)
	if err != nil {
		return nil, &proxy.HTTPError{err, 500, "M_UNKNOWN", "Error generating login"}
	}

	response, err := h.makeLoginResponse(userID, expires, nonce)
	if err != nil {
		return nil, &proxy.HTTPError{err, 500, "M_UNKNOWN", "Error generating login"}
	}

	if err := h.db.insertTokens(response.UserID, response.AccessToken, response.RefreshToken); err != nil {
		return nil, &proxy.HTTPError{err, 500, "M_UNKNOWN", "Error persisting login"}
	}

	return response, nil
}

func (h *MatrixLoginHandler) makeLoginResponse(userID string, expires time.Time, nonce string) (*matrixLoginResponse, error) {

	var response matrixLoginResponse

	accessToken, err := macaroon.New([]byte(h.macaroonSecret), "key", h.serverName)
	if err != nil {
		return nil, err
	}
	accessToken.AddFirstPartyCaveat("gen = 1")
	accessToken.AddFirstPartyCaveat(fmt.Sprintf("user_id = %s", userID))
	refreshToken := accessToken.Clone()

	accessToken.AddFirstPartyCaveat("type = access")
	accessToken.AddFirstPartyCaveat(fmt.Sprintf("time < %d", expires.UnixNano()/1000000))

	refreshToken.AddFirstPartyCaveat("type = refresh")
	refreshToken.AddFirstPartyCaveat(fmt.Sprintf("nonce = %s", nonce))

	if response.AccessToken, err = encodeMacaroon(accessToken); err != nil {
		return nil, err
	}

	if response.RefreshToken, err = encodeMacaroon(refreshToken); err != nil {
		return nil, err
	}

	response.HomeServer = h.serverName
	response.UserID = userID

	return &response, nil
}

func encodeMacaroon(m *macaroon.Macaroon) (string, error) {
	macaroonBytes, err := m.MarshalBinary()
	if err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(macaroonBytes), nil
}

func decodeMacaroon(m string) (*macaroon.Macaroon, error) {
	var out macaroon.Macaroon

	macaroonBytes, err := base64.RawURLEncoding.DecodeString(m)
	if err != nil {
		return nil, err
	}

	if err := out.UnmarshalBinary(macaroonBytes); err != nil {
		return nil, err
	}

	return &out, nil
}

func randomBase64(count int) (string, error) {
	randomBytes := make([]byte, count)

	_, err := rand.Read(randomBytes)
	if err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(randomBytes), nil
}
