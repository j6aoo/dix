package dix

import (
	"context"
	"fmt"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/token"
	"github.com/gagliardetto/solana-go/rpc"
)

func Send(from solana.PublicKey, to solana.PublicKey, amount uint64, keypair solana.PrivateKey, rpcURL string) (string, error) {
	client := rpc.New(rpcURL)
	fromATA, _, err := solana.FindAssociatedTokenAddress(from, solana.MustPublicKeyFromBase58(USDCMint))
	if err != nil {
		return "", fmt.Errorf("from ATA: %w", err)
	}

	toATA, _, err := solana.FindAssociatedTokenAddress(to, solana.MustPublicKeyFromBase58(USDCMint))
	if err != nil {
		return "", fmt.Errorf("to ATA: %w", err)
	}

	transferIx := token.NewTransferInstruction(
		amount,
		fromATA,
		toATA,
		from,
		[]solana.PublicKey{},
	).Build()

	recent, err := client.GetLatestBlockhash(context.Background(), rpc.CommitmentFinalized)
	if err != nil {
		return "", fmt.Errorf("blockhash: %w", err)
	}
	tx, err := solana.NewTransaction(
		[]solana.Instruction{transferIx},
		recent.Value.Blockhash,
		solana.TransactionPayer(from),
	)
	if err != nil {
		return "", fmt.Errorf("tx: %w", err)
	}

	_, err = tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
		if key.Equals(from) {
			return &keypair
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("sign: %w", err)
	}

	sig, err := client.SendTransaction(context.Background(), tx)
	if err != nil {
		return "", fmt.Errorf("send: %w", err)
	}

	return sig.String(), nil
}

func Confirm(sig string, rpcURL string, timeout time.Duration) error {
	client := rpc.New(rpcURL)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	signature := solana.MustSignatureFromBase58(sig)

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for confirmation")
		default:
			status, err := client.GetSignatureStatuses(ctx, false, signature)
			if err == nil && len(status.Value) > 0 && status.Value[0] != nil {
				if status.Value[0].ConfirmationStatus == rpc.ConfirmationStatusFinalized ||
					status.Value[0].ConfirmationStatus == rpc.ConfirmationStatusConfirmed {
					return nil
				}
			}
			time.Sleep(200 * time.Millisecond)
		}
	}
}

func Balance(pubkey solana.PublicKey, rpcURL string) (uint64, error) {
	client := rpc.New(rpcURL)

	ata, _, err := solana.FindAssociatedTokenAddress(pubkey, solana.MustPublicKeyFromBase58(USDCMint))
	if err != nil {
		return 0, err
	}

	result, err := client.GetTokenAccountBalance(context.Background(), ata, rpc.CommitmentFinalized)
	if err != nil {
		return 0, err
	}

	var amount uint64
	fmt.Sscanf(result.Value.Amount, "%d", &amount)
	return amount, nil
}

func SolBalance(pubkey solana.PublicKey, rpcURL string) (uint64, error) {
	client := rpc.New(rpcURL)

	result, err := client.GetBalance(context.Background(), pubkey, rpc.CommitmentFinalized)
	if err != nil {
		return 0, err
	}

	return result.Value, nil
}
