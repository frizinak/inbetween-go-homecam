package protocol

import (
	"bytes"
	"crypto/rand"
	"crypto/sha512"
	"errors"
	"io"

	"github.com/frizinak/inbetween-go-homecam/crypto"
	"golang.org/x/crypto/scrypt"
)

var (
	ErrInvalidHandshake = errors.New("Invalid handshake")
	ErrDenied           = errors.New("Server denied access")
)

var (
	nok = []byte{0}
	ok  = []byte{1}
)

type Protocol struct {
	handshakeCost  uint8
	encryptionCost uint8
	saltSize       int
	hashLen        int
}

func New(handshakeCost, encryptionCost uint8, saltSize, hashLen int) *Protocol {
	return &Protocol{
		handshakeCost:  handshakeCost,
		encryptionCost: encryptionCost,
		saltSize:       saltSize,
		hashLen:        hashLen,
	}
}

func (p *Protocol) HandshakeServer(pass []byte, rw io.ReadWriter) (*crypto.ImmutableKeyEncrypter, error) {
	handshake := make([]byte, p.saltSize)
	if _, err := rand.Read(handshake); err != nil {
		return nil, err
	}

	if _, err := rw.Write(handshake); err != nil {
		return nil, err
	}

	remoteHandshakeHash := make([]byte, p.hashLen)
	if _, err := io.ReadFull(rw, remoteHandshakeHash); err != nil {
		return nil, err
	}

	encryptionPass, handshakeHash, err := p.handshake(pass, handshake)
	if err != nil {
		return nil, err
	}

	if !bytes.Equal(remoteHandshakeHash, handshakeHash) {
		rw.Write(nok)
		return nil, ErrInvalidHandshake
	}

	rw.Write(ok)
	return crypto.NewImmutableKeyEncrypter(encryptionPass, 60, p.encryptionCost)
}

func (p *Protocol) HandshakeClient(pass []byte, rw io.ReadWriter) (*crypto.ImmutableKeyDecrypter, error) {
	handshake := make([]byte, p.saltSize)
	if _, err := io.ReadFull(rw, handshake); err != nil {
		return nil, err
	}

	decryptionPass, handshakeHash, err := p.handshake(pass, handshake)
	if err != nil {
		return nil, err
	}

	if _, err = rw.Write(handshakeHash); err != nil {
		return nil, err
	}

	status := make([]byte, 1)
	if _, err = rw.Read(status); err != nil {
		return nil, err
	}
	if !bytes.Equal(ok, status) {
		return nil, ErrDenied
	}

	return crypto.NewImmutableKeyDecrypter(decryptionPass), nil
}

func (p *Protocol) handshake(pass, salt []byte) (key, handshakeHash []byte, err error) {
	hash := sha512.Sum512(pass)
	handshakeHash, err = scrypt.Key(
		hash[:],
		salt,
		1<<p.handshakeCost,
		8,
		1,
		p.hashLen,
	)
	if err != nil {
		return
	}

	key = append(handshakeHash, pass...)
	return
}
