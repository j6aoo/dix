package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"dix"

	"github.com/gagliardetto/solana-go"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	homeDir, _ = os.UserHomeDir()
	configDir  = filepath.Join(homeDir, ".dix")
	dbpath     = filepath.Join(configDir, "ledger.db")
	keypath    = filepath.Join(configDir, "keypair.json")
	rpcURL     = dix.DevnetRPC
	programID  = dix.RegistryProgram
	tokenFlag  = "usdc"
)

func main() {
	root := &cobra.Command{
		Use:   "dix",
		Short: "decentralized instant exchange - send crypto via username",
	}

	root.PersistentFlags().StringVar(&rpcURL, "rpc", dix.DevnetRPC, "Solana RPC URL")
	root.PersistentFlags().StringVar(&tokenFlag, "token", "usdc", "token to use: usdc, usdt, btc, ltc")

	root.AddCommand(initCmd())
	root.AddCommand(registerCmd())
	root.AddCommand(payCmd())
	root.AddCommand(ledgerCmd())
	root.AddCommand(balanceCmd())
	root.AddCommand(recoverCmd())
	root.AddCommand(tokensCmd())

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func initCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "create new wallet",
		Run: func(cmd *cobra.Command, args []string) {
			if _, err := os.Stat(keypath); err == nil {
				fmt.Println("wallet exists")
				return
			}

			mnemonic, wallet, err := dix.Generate()
			if err != nil {
				die(err)
			}

			fmt.Println("keypair generated")
			fmt.Printf("pubkey: %s\n\n", wallet.Pubkey)
			fmt.Println("save your seed phrase:")
			fmt.Println(mnemonic)
			fmt.Println("")

			pwd := readpwd("password: ")
			if err := dix.Savewallet(keypath, wallet.Secret, pwd); err != nil {
				die(err)
			}

			db, err := dix.Opendb(dbpath)
			if err != nil {
				die(err)
			}
			db.Close()

			fmt.Printf("saved: %s\n", keypath)
		},
	}
}

func recoverCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "recover",
		Short: "restore wallet from mnemonic",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Print("mnemonic: ")
			reader := bufio.NewReader(os.Stdin)
			mnemonic, _ := reader.ReadString('\n')
			mnemonic = strings.TrimSpace(mnemonic)

			wallet, err := dix.Recover(mnemonic)
			if err != nil {
				die(err)
			}

			fmt.Printf("pubkey: %s\n", wallet.Pubkey)

			pwd := readpwd("password: ")
			if err := dix.Savewallet(keypath, wallet.Secret, pwd); err != nil {
				die(err)
			}

			fmt.Println("wallet recovered")
		},
	}
}

func registerCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "register <username>",
		Short: "register username on-chain",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			username := strings.ToLower(args[0])

			if !dix.IsUsername(username) {
				die(fmt.Errorf("invalid username: use 3-20 lowercase letters/numbers"))
			}

			pwd := readpwd("password: ")
			secret, err := dix.Loadwallet(keypath, pwd)
			if err != nil {
				die(err)
			}

			keypair := dix.ToSolanaKey(secret)
			pubkey := keypair.PublicKey()

			fmt.Printf("registering: %s -> %s\n", username, pubkey.String()[:12]+"...")
			fmt.Printf("rpc: %s\n\n", rpcURL)

			db, err := dix.Opendb(dbpath)
			if err != nil {
				die(err)
			}
			defer db.Close()

			sig, err := dix.Register(db, username, keypair, programID, rpcURL)
			if err != nil {
				die(err)
			}

			fmt.Printf("tx: %s\n", sig[:16]+"...")

			if err := dix.Confirm(sig, rpcURL, 30*time.Second); err != nil {
				die(err)
			}

			fmt.Printf("registered: %s\n", username)
			fmt.Println("cost: ~0.001 SOL")
		},
	}
}

func payCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pay <token> <to> <amount>",
		Short: "send tokens to username or pubkey",
		Long:  "Tokens: usdc, usdt, btc, ltc\nExample: dix pay usdc joao 100",
		Args:  cobra.ExactArgs(3),
		Run: func(cmd *cobra.Command, args []string) {
			token := strings.ToLower(args[0])
			to := args[1]
			amount := parseAmount(args[2], token)

			if _, ok := dix.Tokens[token]; !ok {
				die(fmt.Errorf("token not supported: %s (use: usdc, usdt, btc, ltc)", token))
			}

			pwd := readpwd("password: ")
			secret, err := dix.Loadwallet(keypath, pwd)
			if err != nil {
				die(err)
			}

			keypair := dix.ToSolanaKey(secret)
			from := keypair.PublicKey()

			symbol := dix.GetTokenSymbol(token)
			fmt.Printf("sending: %s %s\n", dix.FmtAmount(amount, token), symbol)
			fmt.Printf("from: %s\n", from.String()[:12]+"...")
			fmt.Printf("to: %s\n", to)
			fmt.Printf("rpc: %s\n\n", rpcURL)

			db, err := dix.Opendb(dbpath)
			if err != nil {
				die(err)
			}
			defer db.Close()

			if err := dix.Pay(db, keypair, to, amount, token, programID, rpcURL); err != nil {
				die(err)
			}
		},
	}
}

func ledgerCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ledger",
		Short: "list transactions",
		Run: func(cmd *cobra.Command, args []string) {
			db, err := dix.Opendb(dbpath)
			if err != nil {
				die(err)
			}
			defer db.Close()

			intents, err := dix.List(db, 20)
			if err != nil {
				die(err)
			}

			if len(intents) == 0 {
				fmt.Println("no transactions")
				return
			}

			fmt.Printf("%-10s | %-12s | %14s | %-6s | %s\n", "ID", "TO", "AMOUNT", "STATUS", "TIME")
			fmt.Println(strings.Repeat("-", 65))

			now := time.Now().Unix()
			for _, i := range intents {
				ago := fmtAgo(now - i.Time)
				token := i.Token
				if token == "" {
					token = "usdc"
				}
				symbol := dix.GetTokenSymbol(token)
				fmt.Printf("%-10s | %-12s | %14s | %-6s | %s\n",
					i.ID[:8],
					truncTo(i.To),
					dix.FmtAmount(i.Amount, token)+" "+symbol,
					i.Status,
					ago,
				)
			}
		},
	}
}

func balanceCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "balance",
		Short: "check token balances",
		Run: func(cmd *cobra.Command, args []string) {
			pwd := readpwd("password: ")
			secret, err := dix.Loadwallet(keypath, pwd)
			if err != nil {
				die(err)
			}

			pubkey := solana.PublicKey(secret[32:64])
			fmt.Printf("pubkey: %s\n", pubkey.String())
			fmt.Printf("rpc: %s\n\n", rpcURL)

			for key, info := range dix.Tokens {
				bal, err := dix.Balance(pubkey, key, rpcURL)
				if err != nil {
					fmt.Printf("%s: (no account)\n", info.Symbol)
				} else {
					fmt.Printf("%s: %s\n", info.Symbol, dix.FmtAmount(bal, key))
				}
			}

			sol, err := dix.SolBalance(pubkey, rpcURL)
			if err != nil {
				fmt.Printf("SOL: (error)\n")
			} else {
				fmt.Printf("SOL: %.6f\n", float64(sol)/1e9)
			}
		},
	}
}

func tokensCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tokens",
		Short: "list supported tokens",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("%-6s | %-6s | %s\n", "KEY", "SYMBOL", "MINT")
			fmt.Println(strings.Repeat("-", 60))
			for key, info := range dix.Tokens {
				fmt.Printf("%-6s | %-6s | %s\n", key, info.Symbol, info.Mint[:16]+"...")
			}
		},
	}
}

func die(err error) {
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}

func readpwd(prompt string) []byte {
	fmt.Print(prompt)
	pwd, _ := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	return pwd
}

func parseAmount(s string, token string) uint64 {
	s = strings.TrimSpace(s)
	decimals := dix.GetTokenDecimals(token)
	multiplier := uint64(1)
	for i := uint8(0); i < decimals; i++ {
		multiplier *= 10
	}

	if strings.Contains(s, ".") {
		parts := strings.Split(s, ".")
		whole, _ := strconv.ParseUint(parts[0], 10, 64)
		frac := parts[1]

		for len(frac) < int(decimals) {
			frac += "0"
		}
		frac = frac[:decimals]

		fracN, _ := strconv.ParseUint(frac, 10, 64)
		return whole*multiplier + fracN
	}

	n, _ := strconv.ParseUint(s, 10, 64)
	return n * multiplier
}

func truncTo(s string) string {
	if dix.IsUsername(s) {
		if len(s) > 12 {
			return s[:12]
		}
		return s
	}
	if len(s) > 12 {
		return s[:8] + "..."
	}
	return s
}

func fmtAgo(secs int64) string {
	if secs < 60 {
		return fmt.Sprintf("%ds ago", secs)
	}
	if secs < 3600 {
		return fmt.Sprintf("%dm ago", secs/60)
	}
	if secs < 86400 {
		return fmt.Sprintf("%dh ago", secs/3600)
	}
	return fmt.Sprintf("%dd ago", secs/86400)
}
