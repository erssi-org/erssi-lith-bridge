package erssi

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"fmt"

	"golang.org/x/crypto/pbkdf2"
)

const (
	// Encryption constants from fe-web-crypto.h
	keySize        = 32 // AES-256
	ivSize         = 12 // GCM IV
	tagSize        = 16 // GCM tag
	pbkdf2Iterations = 10000
	pbkdf2Salt     = "irssi-fe-web-v1"
)

// deriveKey derives AES-256 key from password using PBKDF2
func deriveKey(password string) []byte {
	return pbkdf2.Key(
		[]byte(password),
		[]byte(pbkdf2Salt),
		pbkdf2Iterations,
		keySize,
		sha256.New,
	)
}

// decryptMessage decrypts AES-256-GCM encrypted message
// Format: [IV (12 bytes)] [Ciphertext] [Tag (16 bytes)]
func decryptMessage(encrypted []byte, key []byte) ([]byte, error) {
	// Minimum size: IV + Tag
	if len(encrypted) < ivSize+tagSize {
		return nil, fmt.Errorf("encrypted data too short: %d bytes", len(encrypted))
	}

	// Extract components
	iv := encrypted[:ivSize]
	ciphertext := encrypted[ivSize : len(encrypted)-tagSize]
	tag := encrypted[len(encrypted)-tagSize:]

	// Create cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Combine ciphertext + tag for GCM
	sealed := append(ciphertext, tag...)

	// Decrypt
	plaintext, err := gcm.Open(nil, iv, sealed, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}

	return plaintext, nil
}
