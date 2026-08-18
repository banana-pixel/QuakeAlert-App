// Package crypto menyediakan enkripsi/dekripsi AES-256-GCM untuk secret HMAC node.
// secret_key_enc di DB = ciphertext GCM; secret_key_nonce = nonce 12 byte.
// Lihat ADR-0003 & contracts/db/migrations/000001_init_schema.up.sql.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
)

// NonceSize adalah panjang nonce standar GCM (12 byte).
const NonceSize = 12

// Cipher membungkus AES-256-GCM dengan kunci master tetap.
type Cipher struct {
	aead cipher.AEAD
}

// New membuat Cipher dari kunci master 32 byte (AES-256).
func New(masterKey [32]byte) (*Cipher, error) {
	block, err := aes.NewCipher(masterKey[:])
	if err != nil {
		return nil, fmt.Errorf("aes.NewCipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("cipher.NewGCM: %w", err)
	}
	return &Cipher{aead: aead}, nil
}

// Encrypt mengenkripsi plaintext, mengembalikan (ciphertext, nonce).
// Nonce di-generate acak per operasi (WAJIB unik per kunci untuk GCM).
func (c *Cipher) Encrypt(plaintext []byte) (ciphertext, nonce []byte, err error) {
	nonce = make([]byte, NonceSize)
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, fmt.Errorf("generate nonce: %w", err)
	}
	ciphertext = c.aead.Seal(nil, nonce, plaintext, nil)
	return ciphertext, nonce, nil
}

// Decrypt mendekripsi ciphertext dengan nonce yang sesuai.
func (c *Cipher) Decrypt(ciphertext, nonce []byte) ([]byte, error) {
	if len(nonce) != NonceSize {
		return nil, errors.New("nonce harus 12 byte")
	}
	plaintext, err := c.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		// Jangan bocorkan detail; kegagalan Open = tampering / kunci salah.
		return nil, errors.New("dekripsi gagal (autentikasi GCM tidak valid)")
	}
	return plaintext, nil
}
