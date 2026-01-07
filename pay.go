package dix

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/gagliardetto/solana-go"
)


func Pay(db *sql.DB, keypair solana.PrivateKey, to string, amount uint64, programID, rpcURL string) error {
	from := keypair.PublicKey()
	now := time.Now()


	i := Intent{
		ID:     mkid(from.String(), to, amount, now.Unix()),
		From:   from.String(),
		To:     to,
		Amount: amount,
		Time:   now.Unix(),
		Status: "pending",
	}

	existing, err := Load(db, i.ID)
	if err == nil {
		fmt.Printf("intent %s exists (status: %s)\n", existing.ID[:8], existing.Status)
		return nil
	}

	if err := Save(db, i); err != nil {
		return fmt.Errorf("save: %w", err)
	}
	fmt.Printf("✓ intent: %s\n", i.ID[:8])

	var toPubkey solana.PublicKey
	if IsUsername(to) {
		fmt.Printf("✓ resolving %s...\n", to)
		toPubkey, err = Resolve(db, to, programID, rpcURL)
		if err != nil {
			i.Status = "fail"
			Save(db, i)
			return fmt.Errorf("resolve: %w", err)
		}
		fmt.Printf("✓ %s → %s\n", to, toPubkey.String()[:8]+"...")
		i.ToResolved = toPubkey.String()
	} else {
		toPubkey, err = solana.PublicKeyFromBase58(to)
		if err != nil {
			i.Status = "fail"
			Save(db, i)
			return fmt.Errorf("invalid pubkey: %w", err)
		}
		i.ToResolved = to
	}

	if amount == 0 {
		i.Status = "fail"
		Save(db, i)
		return fmt.Errorf("amount cannot be zero")
	}

	start := time.Now()
	sig, err := Send(from, toPubkey, amount, keypair, rpcURL)
	if err != nil {
		i.Status = "fail"
		Save(db, i)
		return fmt.Errorf("send: %w", err)
	}

	i.Signature = sig
	i.Status = "sent"
	Save(db, i)
	fmt.Printf("✓ tx: %s\n", sig[:16]+"...")

	if err := Confirm(sig, rpcURL, 30*time.Second); err != nil {
		i.Status = "fail"
		Save(db, i)
		return fmt.Errorf("confirm: %w", err)
	}

	elapsed := time.Since(start)
	i.Status = "done"
	Save(db, i)

	fmt.Printf("✓ confirmed (%dms)\n", elapsed.Milliseconds())
	fmt.Printf("%s USDC → %s\n", fmtAmount(amount), to)

	return nil
}

func mkid(from, to string, amt uint64, ts int64) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%d:%d", from, to, amt, ts)))
	return hex.EncodeToString(h[:])[:16]
}

func fmtAmount(amt uint64) string {
	whole := amt / 1_000_000
	frac := amt % 1_000_000
	if frac == 0 {
		return fmt.Sprintf("%d", whole)
	}
	return fmt.Sprintf("%d.%06d", whole, frac)
}
