package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"

	"golang.org/x/crypto/scrypt"
)

// Generates an 8-byte key that can be used in config
func main() {
	salt := make([]byte, 8)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		panic("rand reader err: " + err.Error())
	}
	fmt.Println("salt:\t", hex.EncodeToString(salt))

	pass := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, pass); err != nil {
		panic("rand reader err: " + err.Error())
	}
	fmt.Println("pass:\t", hex.EncodeToString(pass))

	key, err := scrypt.Key(pass, salt, 32768, 8, 1, 32)
	if err != nil {
		panic("scrypt err: " + err.Error())
	}

	fmt.Println("key:\t", hex.EncodeToString(key))
}
