package sixty6

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

var SecretKey []byte

func SetCookie(w http.ResponseWriter, c *http.Cookie,
    v map[string]interface{}) error {
	var buf bytes.Buffer
	var b []byte
	var err error

	// Check SecretKey
	if len(SecretKey) == 0 {
		return fmt.Errorf("sixty6.SecretKey is empty, set it to use " +
		    "Cookies.")
	}

	// Serialize with json
	b, err = json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = buf.Write(b)
	if err != nil {
		return err
	}

	// Add a Time Stamp
	err = binary.Write(&buf, binary.LittleEndian, time.Now().Unix())
	if err != nil {
		return fmt.Errorf("Can't write timestamp to cookie.")
	}

	// Generates signature
	sig := hmac.New(sha1.New, SecretKey)
	_, err = sig.Write(buf.Bytes())
	if err != nil {
		return fmt.Errorf("Can't generate signature.")
	}
	bsig := sig.Sum(nil)

	// Add signature
	_, err = buf.Write(bsig)
	if err != nil {
		return fmt.Errorf("Can't write signature to buffer.")
	}

	// Encode to base64
	c.Value = base64.StdEncoding.EncodeToString(buf.Bytes())
	http.SetCookie(w, c)
	return nil
}

func GetCookie(r *http.Request, name string) (map[string]interface{}, error) {
	var buf []byte
	var res = make(map[string]interface{})
	var c *http.Cookie

	// Get cookie
	c, err := r.Cookie(name)
	if err != nil || c.Value == "" {
		return nil, http.ErrNoCookie
	}

	// Decode base64
	buf, err = base64.StdEncoding.DecodeString(c.Value)
	if err != nil {
		log.Println("Error trying to decode base64.")
		return nil, err
	}

	// The hmac_sha1 + int64 sizes
	size := len(buf) - 20
	objSize := size - 8

	// Generate Signature
	sig := hmac.New(sha1.New, SecretKey)
	_, err = sig.Write(buf[:size])
	if err != nil {
		log.Println("Can't generate signature.")
		return nil, err
	}
	bsig := sig.Sum(nil)

	// Check signature
	if !hmac.Equal(bsig, buf[size:]) {
		log.Println("Signature does not match.")
		return nil, nil
	}

	// Deserialize with json
	err = json.Unmarshal(buf[:objSize], &res)
	if err != nil {
		log.Println("Error trying to deserialize json.")
		return nil, err
	}
	return res, nil
}
