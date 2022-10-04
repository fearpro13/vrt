package digest

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
)

type Auth struct {
	Username string
	Realm    string
	Password string
	Method   string
	Uri      string
	Nonce    string
	Tried    bool
	Failed   bool
}

func (auth *Auth) GetHeader() string {
	return "Authorization: " + auth.GetString()
}

func (auth *Auth) GetString() string {
	ha1 := md5Hash(fmt.Sprintf("%s:%s:%s", auth.Username, auth.Realm, auth.Password))
	ha2 := md5Hash(fmt.Sprintf("%s:%s", auth.Method, auth.Uri))
	response := md5Hash(fmt.Sprintf("%s:%s:%s", ha1, auth.Nonce, ha2))
	return fmt.Sprintf(`Digest username="%s", realm="%s", nonce="%s", uri="%s", response="%s"`,
		auth.Username, auth.Realm, auth.Nonce, auth.Uri, response)
}

func md5Hash(response string) string {
	hash := md5.New()
	hash.Write([]byte(response))
	return hex.EncodeToString(hash.Sum(nil))
}
