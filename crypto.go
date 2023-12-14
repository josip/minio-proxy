package minioproxy

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
)

const ENC_BUFFER_SIZE int = 4096
const IV_SIZE int = 16
const HMAC_SIZE int = 32
const ENC_META_SIZE int = IV_SIZE + HMAC_SIZE

var ErrTamperedFile = errors.New("file has been tampered")

func genIv() ([]byte, error) {
	iv := make([]byte, IV_SIZE)
	if _, err := rand.Read(iv); err != nil {
		return nil, err
	}
	return iv, nil
}

// Encrypts a file stream using by:
//  1. Open a HMAC writer with provided key
//  2. Generating a random IV and writing it in cleartext to the output and HMAC
//  3. Reading the file in blocks of ENC_BUFFER_SIZE, encrpyting it with AES-256 in CTR mode,
//     and finally writing the the encryted text to output file and HMAC.
//  4. Writing HMAC sum to the output
//
// In case of an error the reader will be prematurley closed with a non-io.EOF error.
//
// File format:
// [random iv: 16b][encrypted content: variable, same size as original content][hmac sum: 32b]
func encryptStream(encKey []byte, hmacKey []byte, input io.Reader) io.Reader {
	r, w := io.Pipe()

	go func() {
		iv, err := genIv()
		if err != nil {
			w.CloseWithError(errors.Join(errors.New("failed to create iv"), err))
			return
		}

		aes, err := aes.NewCipher(encKey)
		if err != nil {
			w.CloseWithError(errors.Join(errors.New("failed to create aes cipher"), err))
			return
		}

		ctr := cipher.NewCTR(aes, iv)
		sig := hmac.New(sha256.New, hmacKey)

		w.Write(iv)
		sig.Write(iv)

		buf := make([]byte, ENC_BUFFER_SIZE)
		for {
			n, err := input.Read(buf)
			if err != nil && err != io.EOF {
				w.CloseWithError(errors.Join(errors.New("failed to read source file"), err))
				return
			}

			outBuf := make([]byte, n)
			ctr.XORKeyStream(outBuf, buf[:n])
			sig.Write(outBuf)
			w.Write(outBuf)

			if err == io.EOF {
				break
			}
		}

		mac := sig.Sum(nil)
		w.Write(mac)
		// log.Printf("data encrypted with [%x, %x]", iv, mac)
		w.Close()
	}()

	return r
}

// Streams are decrypted in a two step process:
// 1. stream is decrypted into a temporary file to recalculate its HMAC
// 2. if the HMAC is correct, the temporary, now in cleartext, is written to `dest`
func decryptStream(encKey []byte, hmacKey []byte, input io.Reader, fileSize int64, dest io.Writer) error {
	// step 1 Read IV used for AES
	iv := make([]byte, IV_SIZE)
	if n, err := input.Read(iv); n != IV_SIZE || err != nil {
		return err
	}

	cip, err := aes.NewCipher(encKey)
	if err != nil {
		return err
	}
	ctr := cipher.NewCTR(cip, iv)
	sum := hmac.New(sha256.New, hmacKey)
	sum.Write(iv)

	tmp, err := os.CreateTemp("", "minioproxy-dec")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	buf := make([]byte, ENC_BUFFER_SIZE)
	read := int64(IV_SIZE)
	var hmacStartsAt = fileSize - int64(HMAC_SIZE)
	var storedMac []byte

	// step 2 - read encrypted file
	for {
		n, err := input.Read(buf)
		if err != nil && err != io.EOF {
			fmt.Println("error step 2", err)
			return err
		}
		read += int64(n)
		if read > hmacStartsAt {
			overflow := int(read - hmacStartsAt)
			// because we're reusing `buf` we're using `n` as the limit
			n -= overflow
			storedMac = buf[n:(n + HMAC_SIZE)]
			err = io.EOF
		}

		// 2.1 - recalculating hmac of encrypted block
		sum.Write(buf[:n])

		outBuf := make([]byte, n)
		ctr.XORKeyStream(outBuf, buf[:n])

		// 2.2 - write cleartext content into temp file
		if _, err := tmp.Write(outBuf); err != nil && err != io.EOF {
			return err
		}

		if err == io.EOF {
			break
		}
	}

	// step 3 - validate hmac sums match
	remac := sum.Sum(nil)
	if !hmac.Equal(storedMac, remac) {
		log.Printf("data has been tampered with, expected HMAC %x, got %x with iv %x\n",
			storedMac, remac, iv)

		return ErrTamperedFile
	}

	// step 4 - stream temp file to client
	tmp.Sync()
	tmp.Seek(0, io.SeekStart)
	if _, err = io.Copy(dest, tmp); err != nil && err != io.EOF {
		return err
	}

	return nil
}
