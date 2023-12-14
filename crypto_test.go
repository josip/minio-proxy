package minioproxy

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"testing"
	"testing/iotest"
)

func genRandBytes(size int) []byte {
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		panic("can't read random")
	}

	return b
}

func decryptToBuffer(aesKey []byte, hmacKey []byte, input io.Reader, fileSize int64) ([]byte, error) {
	var decrypted bytes.Buffer
	w := bufio.NewWriter(&decrypted)
	if err := decryptStream(aesKey, hmacKey, input, fileSize, w); err != nil {
		return nil, err
	}

	return decrypted.Bytes(), nil
}

func TestEncryptStream(t *testing.T) {
	aesKey := genRandBytes(32)
	hmacKey := genRandBytes(32)

	fileContents := []byte("hello world")
	fileReader := bytes.NewReader(fileContents)

	out := encryptStream(aesKey, hmacKey, fileReader)
	encrypted, err := io.ReadAll(out)
	if err != nil {
		t.Error("should succeed without error", err)
	}
	encryptedLen := len(encrypted)
	expectedLen := len(fileContents) + ENC_META_SIZE
	if encryptedLen != expectedLen {
		t.Error("expected encrypted string to be", expectedLen, "got", encryptedLen)
	}
}

func encryptDecrypt(name string, fileContents []byte) error {
	aesKey := genRandBytes(32)
	hmacKey := genRandBytes(32)

	fileReader := bytes.NewReader(fileContents)
	encrypted := encryptStream(aesKey, hmacKey, fileReader)
	fileSize := int64(len(fileContents) + ENC_META_SIZE)

	decrypted, err := decryptToBuffer(aesKey, hmacKey, encrypted, fileSize)
	if err != nil {
		return errors.Join(errors.New("expected decryption to succeed"), err)
	}

	if !bytes.Equal(fileContents, decrypted) {
		return fmt.Errorf("expected %x after decryption, got %x", fileContents, decrypted)
	}

	return nil
}

func TestDecrypt(t *testing.T) {
	cases := map[string][]byte{
		"empty":          {},
		"hello-world":    []byte("hello world"),
		"same-as-buffer": genRandBytes(ENC_BUFFER_SIZE),
		"buffer-1":       genRandBytes(ENC_BUFFER_SIZE + 1),
		"buffer+1":       genRandBytes(ENC_BUFFER_SIZE - 1),
		"buffer-meta":    genRandBytes(ENC_BUFFER_SIZE - ENC_META_SIZE),
		"buffer-meta-1":  genRandBytes(ENC_BUFFER_SIZE - ENC_META_SIZE - 1),
		"buffer-meta+1":  genRandBytes(ENC_BUFFER_SIZE - ENC_META_SIZE + 1),
		"buffer-iv":      genRandBytes(ENC_BUFFER_SIZE - IV_SIZE),
		"buffer-iv-1":    genRandBytes(ENC_BUFFER_SIZE - IV_SIZE - 1),
		"buffer-iv+1":    genRandBytes(ENC_BUFFER_SIZE - IV_SIZE + 1),
		"buffer-hmac":    genRandBytes(ENC_BUFFER_SIZE - HMAC_SIZE),
		"buffer-hmac-1":  genRandBytes(ENC_BUFFER_SIZE - HMAC_SIZE - 1),
		"buffer-hmac+1":  genRandBytes(ENC_BUFFER_SIZE - HMAC_SIZE + 1),
	}

	for k, v := range cases {
		if err := encryptDecrypt(k, v); err != nil {
			t.Error("case", k, "failed:", err)
		}
	}
}

func TestTamper(t *testing.T) {
	aesKey := genRandBytes(32)
	hmacKey := genRandBytes(32)

	fileContents := []byte("hello world")
	fileReader := bytes.NewReader(fileContents)

	out := encryptStream(aesKey, hmacKey, fileReader)
	encrypted, _ := io.ReadAll(out)
	encrypted[4] += 1
	tampered := bytes.NewReader(encrypted)

	fileSize := int64(len(fileContents) + ENC_META_SIZE)
	decrypted, err := decryptToBuffer(aesKey, hmacKey, tampered, fileSize)
	if decrypted != nil || err == nil {
		t.Error("expected decrypt to fail with tampered file, instead got", decrypted)
	}
}

func TestWrongKeys(t *testing.T) {
	aesKey := genRandBytes(32)
	hmacKey := genRandBytes(32)
	wrongAesKey := genRandBytes(32)
	wrongHmacKey := genRandBytes(32)

	fileContents := []byte("hello world")
	fileReader := bytes.NewReader(fileContents)
	fileSize := int64(len(fileContents) + ENC_META_SIZE)

	cases := [][2][]byte{
		{wrongAesKey, hmacKey},
		{aesKey, wrongHmacKey},
		{wrongAesKey, wrongHmacKey},
	}

	for _, pair := range cases {
		fileReader.Seek(0, io.SeekStart)
		encrypted := encryptStream(aesKey, hmacKey, fileReader)
		decrypted, _ := decryptToBuffer(pair[0], pair[1], encrypted, fileSize)
		if bytes.Equal(fileContents, decrypted) {
			t.Error("expected decrypt to fail with wrong keys, got instead", decrypted)
		}
	}
}

func TestInvalidKeys(t *testing.T) {
	invalidAesKey := genRandBytes(5)
	invalidHmacKey := genRandBytes(5)

	encrypted := encryptStream(invalidAesKey, invalidHmacKey, bytes.NewReader([]byte("hello")))
	if _, err := io.ReadAll(encrypted); err == nil {
		t.Error("expected encryption to fail with invalid AES key")
	}
}

func TestFileReadError(t *testing.T) {
	aesKey := genRandBytes(32)
	hmacKey := genRandBytes(32)

	encrypted := encryptStream(aesKey, hmacKey, iotest.ErrReader(errors.New("random error")))
	if _, err := io.ReadAll(encrypted); err == nil {
		t.Error("expected encryption to fail when input reader fails too")
	}
}
