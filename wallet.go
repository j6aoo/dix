package dix

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/gagliardetto/solana-go"
	"github.com/mr-tron/base58"
	"github.com/tyler-smith/go-bip39"
)

func Generate() (mnemonic string, wallet Wallet, err error) {
	entropy, err := bip39.NewEntropy(256)
	if err != nil {
		return "", Wallet{}, err
	}

	mnemonic, err = bip39.NewMnemonic(entropy)
	if err != nil {
		return "", Wallet{}, err
	}

	seed := bip39.NewSeed(mnemonic, "")

	priv := ed25519.NewKeyFromSeed(seed[:32])
	pub := priv.Public().(ed25519.PublicKey)

	secret := make([]byte, 64)
	copy(secret[:32], seed[:32])
	copy(secret[32:], pub)

	wallet = Wallet{
		Pubkey: base58.Encode(pub),
		Secret: secret,
	}

	return mnemonic, wallet, nil
}

func Recover(mnemonic string) (Wallet, error) {
	if !bip39.IsMnemonicValid(mnemonic) {
		return Wallet{}, errors.New("invalid mnemonic")
	}

	seed := bip39.NewSeed(mnemonic, "")
	priv := ed25519.NewKeyFromSeed(seed[:32])
	pub := priv.Public().(ed25519.PublicKey)

	secret := make([]byte, 64)
	copy(secret[:32], seed[:32])
	copy(secret[32:], pub)

	return Wallet{
		Pubkey: base58.Encode(pub),
		Secret: secret,
	}, nil
}

func Savewallet(path string, secret, password []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	enc, err := encrypt(secret, password)
	if err != nil {
		return err
	}

	data, err := json.Marshal(map[string]string{
		"keypair": hex.EncodeToString(enc),
	})
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}

func Loadwallet(path string, password []byte) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var stored map[string]string
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil, err
	}

	enc, err := hex.DecodeString(stored["keypair"])
	if err != nil {
		return nil, err
	}

	return decrypt(enc, password)
}

func Pubkey(secret []byte) string {
	if len(secret) < 64 {
		return ""
	}
	return base58.Encode(secret[32:64])
}

func ToSolanaKey(secret []byte) solana.PrivateKey {
	return solana.PrivateKey(secret)
}

func encrypt(data, password []byte) ([]byte, error) {
	key := padkey(password)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	return gcm.Seal(nonce, nonce, data, nil), nil
}

func decrypt(data, password []byte) ([]byte, error) {
	key := padkey(password)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	if len(data) < gcm.NonceSize() {
		return nil, errors.New("ciphertext too short")
	}

	nonce := data[:gcm.NonceSize()]
	ciphertext := data[gcm.NonceSize():]

	return gcm.Open(nil, nonce, ciphertext, nil)
}

func padkey(p []byte) []byte {
	k := make([]byte, 32)
	copy(k, p)
	return k
}
