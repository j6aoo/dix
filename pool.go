package dix

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/gagliardetto/solana-go"
)

func CreatePool(db *sql.DB, name, token string, contribution uint64, creator string) (Pool, error) {
	if _, ok := Tokens[token]; !ok {
		return Pool{}, fmt.Errorf("token not supported: %s", token)
	}

	now := time.Now().Unix()
	id := mkPoolID(name, creator, now)

	p := Pool{
		ID:           id,
		Name:         name,
		Token:        token,
		Contribution: contribution,
		Members:      []string{creator},
		Round:        0,
		CreatedAt:    now,
		Status:       "open",
	}

	err := SavePool(db, p)
	if err != nil {
		return Pool{}, err
	}

	err = AddPoolMember(db, id, creator, "", 0)
	if err != nil {
		return Pool{}, err
	}

	return p, nil
}

func JoinPool(db *sql.DB, poolID, username, pubkey string) error {
	p, err := LoadPool(db, poolID)
	if err != nil {
		return fmt.Errorf("pool not found: %s", poolID)
	}

	if p.Status != "open" {
		return fmt.Errorf("pool not open")
	}

	for _, m := range p.Members {
		if m == username {
			return fmt.Errorf("already in pool")
		}
	}

	members, err := ListPoolMembers(db, poolID)
	if err != nil {
		return err
	}

	order := len(members)
	return AddPoolMember(db, poolID, username, pubkey, order)
}

func StartPool(db *sql.DB, poolID string) error {
	p, err := LoadPool(db, poolID)
	if err != nil {
		return err
	}

	members, err := ListPoolMembers(db, poolID)
	if err != nil {
		return err
	}

	if len(members) < 2 {
		return fmt.Errorf("need at least 2 members")
	}

	p.Status = "active"
	p.Round = 1
	return SavePool(db, p)
}

func ContributePool(db *sql.DB, poolID, username string, keypair solana.PrivateKey, rpcURL string) error {
	p, err := LoadPool(db, poolID)
	if err != nil {
		return err
	}

	if p.Status != "active" {
		return fmt.Errorf("pool not active")
	}

	member, err := GetPoolMember(db, poolID, username)
	if err != nil {
		return fmt.Errorf("not a member")
	}

	if member.Paid {
		return fmt.Errorf("already paid this round")
	}

	winner, err := GetRoundWinner(db, poolID, p.Round)
	if err != nil {
		return fmt.Errorf("no winner for round %d", p.Round)
	}

	winnerPubkey, err := solana.PublicKeyFromBase58(winner.Pubkey)
	if err != nil {
		return err
	}

	from := keypair.PublicKey()
	sig, err := Send(from, winnerPubkey, p.Contribution, p.Token, keypair, rpcURL)
	if err != nil {
		return fmt.Errorf("send: %w", err)
	}

	fmt.Printf("tx: %s\n", sig[:16]+"...")

	err = Confirm(sig, rpcURL, 30*time.Second)
	if err != nil {
		return err
	}

	return MarkPaid(db, poolID, username)
}

func ClaimPool(db *sql.DB, poolID, username string) error {
	p, err := LoadPool(db, poolID)
	if err != nil {
		return err
	}

	winner, err := GetRoundWinner(db, poolID, p.Round)
	if err != nil {
		return err
	}

	if winner.Username != username {
		return fmt.Errorf("not your turn (winner: %s)", winner.Username)
	}

	members, err := ListPoolMembers(db, poolID)
	if err != nil {
		return err
	}

	allPaid := true
	for _, m := range members {
		if m.Username != username && !m.Paid {
			allPaid = false
			break
		}
	}

	if !allPaid {
		return fmt.Errorf("not everyone paid yet")
	}

	err = MarkClaimed(db, poolID, username)
	if err != nil {
		return err
	}

	return AdvanceRound(db, poolID)
}

func AdvanceRound(db *sql.DB, poolID string) error {
	p, err := LoadPool(db, poolID)
	if err != nil {
		return err
	}

	members, err := ListPoolMembers(db, poolID)
	if err != nil {
		return err
	}

	if p.Round >= len(members) {
		p.Status = "done"
		return SavePool(db, p)
	}

	p.Round++
	err = SavePool(db, p)
	if err != nil {
		return err
	}

	return ResetPaid(db, poolID)
}

func PoolStatus(db *sql.DB, poolID string) (Pool, []PoolMember, error) {
	p, err := LoadPool(db, poolID)
	if err != nil {
		return Pool{}, nil, err
	}

	members, err := ListPoolMembers(db, poolID)
	if err != nil {
		return Pool{}, nil, err
	}

	return p, members, nil
}

func mkPoolID(name, creator string, ts int64) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%d", name, creator, ts)))
	return hex.EncodeToString(h[:])[:12]
}
