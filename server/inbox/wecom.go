package inbox

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	weComMaxBodyBytes  = 64 << 10
	weComMaxQueryBytes = 4 << 10
	weComClockSkew     = 5 * time.Minute
	weComPaddingSize   = 32
	weComAESKeyLength  = 43
	weComFramingSize   = 16 + 4 // Random prefix and a big-endian message length.
)

type weComCallback struct {
	corpID   string
	agentID  int64
	token    string
	block    cipher.Block
	iv       []byte
	senderID string
	save     func(context.Context, string, string) error
}

// NewWeCom creates the encrypted callback handler for one self-built WeCom
// application and one authorized sender. The save function receives the stable
// external ID and text and must be safe to call again for the same message.
func NewWeCom(corpID string, agentID int64, token, encodingAESKey, senderID string, save func(context.Context, string, string) error) (http.Handler, error) {
	if corpID == "" || corpID != strings.TrimSpace(corpID) || strings.Contains(corpID, "/") {
		return nil, errors.New("invalid WeCom corporation ID")
	}
	if agentID <= 0 {
		return nil, errors.New("invalid WeCom application AgentID")
	}
	if token == "" || token != strings.TrimSpace(token) {
		return nil, errors.New("invalid WeCom callback token")
	}
	if senderID == "" || senderID != strings.TrimSpace(senderID) || strings.Contains(senderID, "/") {
		return nil, errors.New("invalid WeCom sender ID")
	}
	if save == nil {
		return nil, errors.New("missing WeCom message handler")
	}
	if len(encodingAESKey) != weComAESKeyLength {
		return nil, errors.New("invalid WeCom EncodingAESKey")
	}
	key, err := base64.RawStdEncoding.Strict().DecodeString(encodingAESKey)
	if err != nil || len(key) != 32 {
		return nil, errors.New("invalid WeCom EncodingAESKey")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, errors.New("invalid WeCom EncodingAESKey")
	}
	return &weComCallback{corpID: corpID, agentID: agentID, token: token, block: block, iv: key[:aes.BlockSize], senderID: senderID, save: save}, nil
}

func (h *weComCallback) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	query, err := parseWeComQuery(r.URL.RawQuery)
	if err != nil {
		http.Error(w, "invalid callback request", http.StatusBadRequest)
		return
	}

	var encrypted string
	if r.Method == http.MethodGet {
		if len(query["echostr"]) != 1 || query.Get("echostr") == "" {
			http.Error(w, "invalid callback request", http.StatusBadRequest)
			return
		}
		encrypted = query.Get("echostr")
	} else {
		encrypted, err = readWeComEnvelope(w, r)
		if err != nil {
			if _, tooLarge := errors.AsType[*http.MaxBytesError](err); tooLarge {
				http.Error(w, "callback body too large", http.StatusRequestEntityTooLarge)
				return
			}
			http.Error(w, "invalid callback request", http.StatusBadRequest)
			return
		}
	}

	if !h.validSignature(query, encrypted) {
		http.Error(w, "invalid callback authentication", http.StatusForbidden)
		return
	}
	message, valid := h.decrypt(encrypted)
	if !valid {
		http.Error(w, "invalid callback authentication", http.StatusForbidden)
		return
	}
	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write(message)
		return
	}
	h.receiveText(w, r, message)
}

func parseWeComQuery(raw string) (url.Values, error) {
	if len(raw) > weComMaxQueryBytes {
		return nil, errors.New("callback query too large")
	}
	query, err := url.ParseQuery(raw)
	if err != nil {
		return nil, err
	}
	for _, key := range []string{"msg_signature", "timestamp", "nonce"} {
		if len(query[key]) != 1 || query.Get(key) == "" {
			return nil, errors.New("missing or repeated callback parameter")
		}
	}
	return query, nil
}

func readWeComEnvelope(w http.ResponseWriter, r *http.Request) (string, error) {
	payload, err := io.ReadAll(http.MaxBytesReader(w, r.Body, weComMaxBodyBytes))
	if err != nil {
		return "", err
	}
	var envelope struct {
		XMLName xml.Name `xml:"xml"`
		Encrypt []string `xml:"Encrypt"`
	}
	if err := decodeWeComXML(payload, &envelope); err != nil {
		return "", err
	}
	if len(envelope.Encrypt) != 1 || envelope.Encrypt[0] == "" {
		return "", errors.New("missing or repeated ciphertext")
	}
	return envelope.Encrypt[0], nil
}

func (h *weComCallback) validSignature(query url.Values, encrypted string) bool {
	timestamp := query.Get("timestamp")
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	now := time.Now().Unix()
	skew := int64(weComClockSkew / time.Second)
	if err != nil || seconds < now-skew || seconds > now+skew {
		return false
	}
	signature, err := hex.DecodeString(query.Get("msg_signature"))
	if err != nil || len(signature) != sha1.Size {
		return false
	}
	parts := []string{h.token, timestamp, query.Get("nonce"), encrypted}
	slices.Sort(parts)
	// WeCom's callback protocol specifies sorted concatenation with SHA-1.
	digest := sha1.Sum([]byte(strings.Join(parts, "")))
	return subtle.ConstantTimeCompare(signature, digest[:]) == 1
}

func (h *weComCallback) decrypt(encrypted string) ([]byte, bool) {
	plaintext, err := base64.StdEncoding.Strict().DecodeString(encrypted)
	if err != nil || len(plaintext) == 0 || len(plaintext)%weComPaddingSize != 0 {
		return nil, false
	}
	cipher.NewCBCDecrypter(h.block, h.iv).CryptBlocks(plaintext, plaintext)
	last := len(plaintext) - 1
	pad := int(plaintext[last])
	if pad < 1 || pad > weComPaddingSize {
		return nil, false
	}
	end := len(plaintext) - pad
	for _, value := range plaintext[end:] {
		if int(value) != pad {
			return nil, false
		}
	}
	if end < weComFramingSize {
		return nil, false
	}
	messageLength := binary.BigEndian.Uint32(plaintext[16:weComFramingSize])
	if uint64(messageLength) > uint64(end-weComFramingSize) {
		return nil, false
	}
	messageEnd := weComFramingSize + int(messageLength)
	if !bytes.Equal(plaintext[messageEnd:end], []byte(h.corpID)) {
		return nil, false
	}
	return plaintext[weComFramingSize:messageEnd], messageLength > 0
}

func (h *weComCallback) receiveText(w http.ResponseWriter, r *http.Request, plaintext []byte) {
	var message struct {
		XMLName    xml.Name `xml:"xml"`
		CorpID     string   `xml:"ToUserName"`
		SenderID   string   `xml:"FromUserName"`
		Type       string   `xml:"MsgType"`
		Content    string   `xml:"Content"`
		ExternalID string   `xml:"MsgId"`
		AgentID    string   `xml:"AgentID"`
	}
	if err := decodeWeComXML(plaintext, &message); err != nil || message.Type == "" {
		http.Error(w, "invalid callback message", http.StatusBadRequest)
		return
	}
	if message.CorpID != "" && message.CorpID != h.corpID {
		http.Error(w, "invalid callback authentication", http.StatusForbidden)
		return
	}
	if message.Type == "text" {
		agentID, err := strconv.ParseInt(message.AgentID, 10, 64)
		if err != nil || agentID <= 0 || message.SenderID == "" {
			http.Error(w, "invalid callback message", http.StatusBadRequest)
			return
		}
		// Scope the message with the agent and sender in the authenticated XML.
		// The outer envelope's routing fields are not covered by the signature.
		if agentID == h.agentID && message.SenderID == h.senderID && strings.TrimSpace(message.Content) != "" {
			id, err := strconv.ParseUint(message.ExternalID, 10, 64)
			if err != nil || id == 0 {
				http.Error(w, "invalid callback message", http.StatusBadRequest)
				return
			}
			externalID := strings.Join([]string{h.corpID, strconv.FormatInt(agentID, 10), h.senderID, strconv.FormatUint(id, 10)}, "/")
			if err := h.save(r.Context(), externalID, message.Content); err != nil {
				http.Error(w, "callback storage unavailable", http.StatusServiceUnavailable)
				return
			}
		}
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, "success")
}

func decodeWeComXML(payload []byte, value any) error {
	decoder := xml.NewDecoder(bytes.NewReader(bytes.TrimPrefix(payload, []byte("\xef\xbb\xbf"))))
	var rootSeen bool
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) && rootSeen {
			return nil
		}
		if err != nil {
			return err
		}
		switch element := token.(type) {
		case xml.StartElement:
			if rootSeen {
				return errors.New("multiple roots in callback XML")
			}
			if err := decoder.DecodeElement(value, &element); err != nil {
				return err
			}
			rootSeen = true
		case xml.CharData:
			if len(bytes.TrimSpace(element)) != 0 {
				return errors.New("unexpected data outside callback XML")
			}
		case xml.Comment, xml.ProcInst:
		default:
			return errors.New("invalid callback XML")
		}
	}
}
