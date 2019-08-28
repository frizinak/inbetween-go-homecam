package crypto

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"

	"golang.org/x/crypto/scrypt"
)

const (
	MaxCost     = 30
	MinCost     = 6
	MinSaltSize = 8
)

const chunkSizeMulti = 50

type KeySlice []byte

type ImmutableKeyEncrypter struct {
	key      KeySlice
	salt     []byte
	saltSize uint16
	cost     uint8
}

func NewImmutableKeyEncrypter(passphrase []byte, saltSize uint16, cost uint8) (*ImmutableKeyEncrypter, error) {
	salt, err := salt(saltSize)
	if err != nil {
		return nil, err
	}

	key, err := Key(passphrase, salt, cost)
	if err != nil {
		return nil, err
	}

	return &ImmutableKeyEncrypter{key: key, salt: salt, saltSize: saltSize, cost: cost}, nil
}

func (e *ImmutableKeyEncrypter) Encrypt(
	r io.Reader,
	w io.Writer,
) error {
	iv := make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return err
	}

	block, err := aes.NewCipher(e.key)
	if err != nil {
		return err
	}

	if err := binary.Write(w, binary.LittleEndian, e.cost); err != nil {
		return err
	}

	if err := binary.Write(w, binary.LittleEndian, e.saltSize); err != nil {
		return err
	}

	if _, err := w.Write(e.salt); err != nil {
		return err
	}

	if _, err := w.Write(iv); err != nil {
		return err
	}

	return encrypt(
		r,
		w,
		block,
		iv,
	)
}

type ImmutableKeyDecrypter struct {
	key        KeySlice
	passphrase []byte
}

func NewImmutableKeyDecrypter(passphrase []byte) *ImmutableKeyDecrypter {
	return &ImmutableKeyDecrypter{passphrase: passphrase}
}

func (d *ImmutableKeyDecrypter) Decrypt(r io.Reader, w io.Writer) error {
	if d.key == nil {
		salt, iv, cost, err := header(r)
		if err != nil {
			return err
		}

		key, err := decryptWithHeader(r, w, d.passphrase, salt, iv, cost)
		if err != nil {
			return err
		}
		d.key = key

		return nil
	}

	return decryptWithKey(r, w, d.key)
}

func Encrypt(
	r io.Reader,
	w io.Writer,
	passphrase []byte,
	saltSize uint16,
	cost uint8,
) error {
	e, err := NewImmutableKeyEncrypter(passphrase, saltSize, cost)
	if err != nil {
		return err
	}
	return e.Encrypt(r, w)
}

func Decrypt(
	r io.Reader,
	w io.Writer,
	passphrase []byte,
) error {
	return NewImmutableKeyDecrypter(passphrase).Decrypt(r, w)
}

func header(r io.Reader) (salt, iv []byte, cost uint8, err error) {
	err = errors.New("Input is not encrypted")
	var saltSize uint16
	if binary.Read(r, binary.LittleEndian, &cost) != nil {
		return
	}

	if binary.Read(r, binary.LittleEndian, &saltSize) != nil {
		return
	}

	header := make([]byte, saltSize+aes.BlockSize)
	if _, errl := io.ReadFull(r, header); errl != nil {
		return
	}

	salt = header[:saltSize]
	iv = header[saltSize:]
	err = nil

	return
}

func decryptWithHeader(
	r io.Reader,
	w io.Writer,
	passphrase []byte,
	salt,
	iv []byte,
	cost uint8,
) (KeySlice, error) {
	key, err := Key(passphrase, salt, cost)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	return key, decrypt(
		r,
		w,
		block,
		iv,
	)
}

func decryptWithKey(r io.Reader, w io.Writer, key KeySlice) error {
	_, iv, _, err := header(r)
	if err != nil {
		return err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}

	return decrypt(
		r,
		w,
		block,
		iv,
	)
}

func encrypt(
	r io.Reader,
	w io.Writer,
	block cipher.Block,
	iv []byte,
) error {
	size := int64(block.BlockSize())
	mode := cipher.NewCBCEncrypter(block, iv)
	chunk := bytes.NewBuffer(nil)
	chunkSize := int64(size * chunkSizeMulti)
	out := make([]byte, chunkSize)

	for {
		n, err := io.CopyN(chunk, r, chunkSize)
		if err != nil && err != io.EOF {
			return err
		}

		if n%size != 0 {
			padN := size - n%size
			chunk.Write(pad(padN))
			n += padN
		} else if err == io.EOF {
			chunk.Write(pad(size))
			n += size
		}

		mode.CryptBlocks(out, chunk.Bytes()[:n])
		if _, err := w.Write(out[:n]); err != nil {
			return err
		}

		chunk.Reset()

		if err == io.EOF {
			break
		}
	}

	return nil
}

func decrypt(
	r io.Reader,
	w io.Writer,
	block cipher.Block,
	iv []byte,
) error {
	size := int64(block.BlockSize())
	sizeb := byte(size)
	mode := cipher.NewCBCDecrypter(block, iv)
	chunk := bytes.NewBuffer(nil)
	chunkSize := int64(size * chunkSizeMulti)
	var prev []byte

	prev = make([]byte, 0, chunkSize)
	out := make([]byte, chunkSize)

	for {
		n, err := io.CopyN(chunk, r, chunkSize)
		if err != nil && err != io.EOF {
			return err
		}

		if n != chunkSize && !(n == 0 || err == io.EOF) {
			return errors.New("Invalid blocksize")
		}

		out = out[:chunkSize]
		mode.CryptBlocks(out, chunk.Bytes()[:n])
		if _, err := w.Write(prev); err != nil {
			return err
		}

		out = out[:n]
		prev = prev[:n]
		copy(prev, out)
		chunk.Reset()

		if err == io.EOF {
			if len(out) == 0 {
				break
			}

			v := out[len(out)-1]
			if v <= sizeb {
				vi := int(v)
				nlen := len(out) - vi
				padded := out[nlen:]
				out = out[:nlen]
				for i := range padded {
					if padded[i] != v {
						out = out[:nlen+vi]
						break
					}
				}
			}

			if _, err := w.Write(out); err != nil {
				return err
			}

			break
		}
	}

	return nil
}

func Key(passphrase, salt []byte, cost uint8) (KeySlice, error) {
	if cost > MaxCost {
		return nil, errors.New("scrypt cost too high")
	} else if cost < MinCost {
		return nil, errors.New("scrypt cost too low")
	}

	if len(salt) < MinSaltSize {
		return nil, errors.New("Salt too short")
	}

	return scrypt.Key(passphrase, salt, 1<<cost, 8, 1, 32)
}

func salt(size uint16) ([]byte, error) {
	salt := make([]byte, size)
	_, err := io.ReadFull(rand.Reader, salt)
	return salt, err
}

func pad(amount int64) []byte {
	data := make([]byte, amount)
	var i int64
	for i = 0; i < amount; i++ {
		data[i] = byte(amount)
	}

	return data
}
