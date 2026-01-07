package dix

import (
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

func IsUsername(s string) bool {
	if len(s) < 3 || len(s) > 20 {
		return false
	}
	if len(s) >= 32 {
		_, err := solana.PublicKeyFromBase58(s)
		if err == nil {
			return false
		}
	}
	for _, c := range s {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_') {
			return false
		}
	}
	return true
}

func Resolve(db *sql.DB, username string, programID, rpcURL string) (solana.PublicKey, error) {
	username = strings.ToLower(username)

	cached, err := Getalias(db, username)
	if err == nil && cached != "" {
		return solana.PublicKeyFromBase58(cached)
	}

	if programID == "" {
		return solana.PublicKey{}, fmt.Errorf("registry program not deployed")
	}

	program := solana.MustPublicKeyFromBase58(programID)
	pda, _, err := solana.FindProgramAddress(
		[][]byte{[]byte("alias"), []byte(username)},
		program,
	)
	if err != nil {
		return solana.PublicKey{}, err
	}

	client := rpc.New(rpcURL)
	acct, err := client.GetAccountInfo(context.Background(), pda)
	if err != nil || acct.Value == nil {
		return solana.PublicKey{}, fmt.Errorf("username not found: %s", username)
	}

	data := acct.Value.Data.GetBinary()
	if len(data) < 40 {
		return solana.PublicKey{}, fmt.Errorf("invalid account data")
	}

	owner := solana.PublicKeyFromBytes(data[8:40])

	Savealias(db, username, owner.String())

	return owner, nil
}

func Register(db *sql.DB, username string, keypair solana.PrivateKey, programID, rpcURL string) (string, error) {
	username = strings.ToLower(username)

	if !IsUsername(username) {
		return "", fmt.Errorf("invalid username: use 3-20 lowercase letters/numbers")
	}

	if programID == "" {
		return "", fmt.Errorf("registry program not deployed")
	}

	client := rpc.New(rpcURL)
	program := solana.MustPublicKeyFromBase58(programID)
	user := keypair.PublicKey()

	pda, bump, err := solana.FindProgramAddress(
		[][]byte{[]byte("alias"), []byte(username)},
		program,
	)
	if err != nil {
		return "", err
	}

	discriminator := []byte{0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	usernameBytes := []byte(username)
	data := make([]byte, 8+4+len(usernameBytes)+1)
	copy(data[:8], discriminator)
	binary.LittleEndian.PutUint32(data[8:12], uint32(len(usernameBytes)))
	copy(data[12:12+len(usernameBytes)], usernameBytes)
	data[12+len(usernameBytes)] = bump

	instruction := solana.NewInstruction(
		program,
		solana.AccountMetaSlice{
			{PublicKey: pda, IsSigner: false, IsWritable: true},
			{PublicKey: user, IsSigner: true, IsWritable: true},
			{PublicKey: solana.SystemProgramID, IsSigner: false, IsWritable: false},
		},
		data,
	)

	recent, err := client.GetLatestBlockhash(context.Background(), rpc.CommitmentFinalized)
	if err != nil {
		return "", err
	}
	tx, err := solana.NewTransaction(
		[]solana.Instruction{instruction},
		recent.Value.Blockhash,
		solana.TransactionPayer(user),
	)
	if err != nil {
		return "", err
	}

	_, err = tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
		if key.Equals(user) {
			return &keypair
		}
		return nil
	})
	if err != nil {
		return "", err
	}

	sig, err := client.SendTransaction(context.Background(), tx)
	if err != nil {
		return "", err
	}
	Savealias(db, username, user.String())

	return sig.String(), nil
}
