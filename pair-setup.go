package hap

import (
	"github.com/brutella/hap/chacha20poly1305"
	"github.com/brutella/hap/ed25519"
	"github.com/brutella/hap/hkdf"
	"github.com/brutella/hap/log"
	"github.com/brutella/hap/tlv8"

	"net/http"
)

const (
	Step1 byte = 0x1
	Step2 byte = 0x2
	Step3 byte = 0x3
	Step4 byte = 0x4
	Step5 byte = 0x5
	Step6 byte = 0x6
)

type pairSetupPayload struct {
	Method        byte   `tlv8:"0"`
	Identifier    string `tlv8:"1"`
	Salt          []byte `tlv8:"2"`
	PublicKey     []byte `tlv8:"3"`
	Proof         []byte `tlv8:"4"`
	EncryptedData []byte `tlv8:"5"`
	State         byte   `tlv8:"6"`
	Error         byte   `tlv8:"7"`
	RetryDelay    byte   `tlv8:"8"`
	Certificate   []byte `tlv8:"9"`
	Signature     []byte `tlv8:"10"`
	Permissions   byte   `tlv8:"11"`
	FragmentData  []byte `tlv8:"13"`
	FragmentLast  []byte `tlv8:"14"`
}

func (srv *Server) pairSetup(res http.ResponseWriter, req *http.Request) {
	// pairing is only allowed if the accessory is not paired yet
	if srv.isPaired() {
		log.Info.Println("pairing is not allowed")
		tlv8Error(res, Step2, TlvErrorUnavailable)
		return
	}

	// pair-setup can only be run by one controller simultaneously
	for addr, _ := range sessions() {
		if addr != req.RemoteAddr {
			log.Info.Printf("simulatenous pairings are not allowed")
			tlv8Error(res, Step2, TlvErrorBusy)
			return
		}
	}

	data := pairSetupPayload{}
	if err := tlv8.UnmarshalReader(req.Body, &data); err != nil {
		log.Info.Println("tlv8:", err)
		res.WriteHeader(http.StatusBadRequest)
		tlv8Error(res, Step2, TlvErrorUnknown)
		return
	}

	switch data.Method {
	case MethodPair:
		switch data.State {
		case Step1:
			srv.pairSetupStep1(res, req, data)
		case Step3:
			srv.pairSetupStep3(res, req, data)
		case Step5:
			srv.pairSetupStep5(res, req, data)
		default:
			log.Info.Println("invalid state", data.State)
			res.WriteHeader(http.StatusBadRequest)
			tlv8Error(res, Step2, TlvErrorUnknown)
		}
	case MethodPairMFi:
		res.WriteHeader(http.StatusBadRequest)
		tlv8Error(res, Step2, TlvErrorInvalidRequest)
	default:
		log.Info.Println("pair setup: invalid method", data.Method)
		res.WriteHeader(http.StatusBadRequest)
		tlv8Error(res, 0, TlvErrorInvalidRequest)
	}
}

type pairSetupStep2Payload struct {
	Salt      []byte `tlv8:"2"`
	PublicKey []byte `tlv8:"3"`
	State     byte   `tlv8:"6"`
}

type pairSetupStep4Payload struct {
	Proof []byte `tlv8:"4"`
	State byte   `tlv8:"6"`
}

type pairSetupStep6EncryptedPayload struct {
	Identifier []byte `tlv8:"1"`
	PublicKey  []byte `tlv8:"3"`
	Signature  []byte `tlv8:"10"`
}

type pairSetupStep6Payload struct {
	EncryptedData []byte `tlv8:"5"`
	State         byte   `tlv8:"6"`
}

func (srv *Server) pairSetupStep1(res http.ResponseWriter, req *http.Request, data pairSetupPayload) {
	// Create a new session.
	ss, err := newPairSetupSession(srv.uuid, srv.fmtPin())
	if err != nil {
		res.WriteHeader(http.StatusInternalServerError)
		tlv8Error(res, data.State+1, TlvErrorUnknown)
		return
	}
	setSession(req.RemoteAddr, ss)

	resp := pairSetupStep2Payload{
		Salt:      ss.Salt,
		PublicKey: ss.PublicKey,
		State:     Step2,
	}
	tlv8OK(res, resp)
}

func (srv *Server) pairSetupStep3(res http.ResponseWriter, req *http.Request, data pairSetupPayload) {
	ses, err := getPairSetupSession(req.RemoteAddr)
	if err != nil {
		log.Info.Println(err)
		res.WriteHeader(http.StatusInternalServerError)
		tlv8Error(res, Step2, TlvErrorUnknown)
		return
	}

	err = ses.SetupPrivateKeyFromClientPublicKey(data.PublicKey)
	if err != nil {
		log.Info.Println(err)
		tlv8Error(res, data.State+1, TlvErrorInvalidRequest)
		return
	}
	proof, err := ses.ProofFromClientProof(data.Proof)
	if err != nil {
		log.Info.Println(err)
		tlv8Error(res, data.State+1, TlvErrorInvalidRequest)
		return
	}

	err = ses.SetupEncryptionKey([]byte("Pair-Setup-Encrypt-Salt"), []byte("Pair-Setup-Encrypt-Info"))
	if err != nil {
		log.Info.Println("pair-setup:", err)
		tlv8Error(res, data.State+1, TlvErrorInvalidRequest)
		return
	}

	resp := pairSetupStep4Payload{
		Proof: proof,
		State: Step4,
	}
	tlv8OK(res, resp)
}

func (srv *Server) pairSetupStep5(res http.ResponseWriter, req *http.Request, data pairSetupPayload) {
	ses, err := getPairSetupSession(req.RemoteAddr)
	if err != nil {
		log.Info.Println(err)
		res.WriteHeader(http.StatusInternalServerError)
		tlv8Error(res, Step6, TlvErrorUnknown)
		return
	}

	msg := data.EncryptedData[:len(data.EncryptedData)-16]
	var mac [16]byte
	copy(mac[:], data.EncryptedData[len(msg):]) // 16 byte (MAC)

	decrypted, err := chacha20poly1305.DecryptAndVerify(ses.EncryptionKey[:], []byte("PS-Msg05"), msg, mac, nil)

	if err != nil {
		res.WriteHeader(http.StatusInternalServerError)
		tlv8Error(res, Step6, TlvErrorUnknown)
		return
	}

	encData := struct {
		Identifier string `tlv8:"1"`
		PublicKey  []byte `tlv8:"3"`
		Signature  []byte `tlv8:"10"`
	}{}
	if err := tlv8.Unmarshal(decrypted, &encData); err != nil {
		log.Info.Println("tlv8:", err)
		res.WriteHeader(http.StatusBadRequest)
		tlv8Error(res, Step6, TlvErrorUnknown)
		return
	}

	log.Debug.Println(toJSON(encData))

	hash, _ := hkdf.Sha512(ses.PrivateKey, []byte("Pair-Setup-Controller-Sign-Salt"), []byte("Pair-Setup-Controller-Sign-Info"))
	var buf []byte
	buf = append(buf, hash[:]...)
	buf = append(buf, encData.Identifier[:]...)
	buf = append(buf, encData.PublicKey[:]...)

	if !ed25519.ValidateSignature(encData.PublicKey[:], buf, encData.Signature) {
		log.Info.Println("ed25519 signature invalid")
		tlv8Error(res, Step6, TlvErrorInvalidRequest)
		return
	}

	log.Debug.Println("ed25519 signature valid")

	hash, err = hkdf.Sha512(ses.PrivateKey, []byte("Pair-Setup-Accessory-Sign-Salt"), []byte("Pair-Setup-Accessory-Sign-Info"))
	if err != nil {
		log.Info.Println(err)
		tlv8Error(res, Step6, TlvErrorInvalidRequest)
		return
	}

	buf = make([]byte, 0)
	buf = append(buf, hash[:]...)
	buf = append(buf, ses.Identifier[:]...)
	buf = append(buf, srv.Key.Public[:]...)

	signature, err := ed25519.Signature(srv.Key.Private[:], buf)
	if err != nil {
		log.Info.Println(err)
		tlv8Error(res, Step6, TlvErrorInvalidRequest)
		return
	}

	privateData := pairSetupStep6EncryptedPayload{
		Identifier: ses.Identifier,
		PublicKey:  srv.Key.Public[:],
		Signature:  signature,
	}
	b, err := tlv8.Marshal(privateData)
	if err != nil {
		log.Info.Println(err)
		tlv8Error(res, Step6, TlvErrorInvalidRequest)
		return
	}

	encrypted, mac, _ := chacha20poly1305.EncryptAndSeal(ses.EncryptionKey[:], []byte("PS-Msg06"), b, nil)

	resp := pairSetupStep6Payload{
		State:         Step6,
		EncryptedData: append(encrypted, mac[:]...),
	}
	tlv8OK(res, resp)

	log.Debug.Println("storing public key for", encData.Identifier)

	p := Pairing{
		Name:       encData.Identifier,
		PublicKey:  encData.PublicKey,
		Permission: PermissionAdmin, // controller is admin by default
	}
	srv.savePairing(p)
}
