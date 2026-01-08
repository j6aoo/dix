package dix

type Intent struct {
	ID         string
	From       string
	To         string
	ToResolved string
	Amount     uint64
	Token      string
	Signature  string
	Time       int64
	Status     string
}

type Wallet struct {
	Pubkey string
	Secret []byte
}

type Alias struct {
	Username string
	Owner    string
}

type Config struct {
	RPC      string
	Keystore string
	DbPath   string
	Program  string
}

const (
	MainnetRPC = "https://api.mainnet-beta.solana.com"
	DevnetRPC  = "https://api.devnet.solana.com"

	RegistryProgram = ""
)

var Tokens = map[string]TokenInfo{
	"usdc": {
		Mint:     "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v",
		Symbol:   "USDC",
		Decimals: 6,
	},
	"usdt": {
		Mint:     "Es9vMFrzaCERmJfrF4H2FYD4KCoNkY11McCe8BenwNYB",
		Symbol:   "USDT",
		Decimals: 6,
	},
	"btc": {
		Mint:     "3NZ9JMVBmGAqocybic2c7LQCJScmgsAZ6vQqTDzcqmJh", // wBTC (Wormhole)
		Symbol:   "wBTC",
		Decimals: 8,
	},
	"ltc": {
		Mint:     "HZRCwxP2Vq9PCpPXooayhJ2bxTpo5xfpQrwB1svh332p", // wLTC (Wormhole)
		Symbol:   "wLTC",
		Decimals: 8,
	},
}

type TokenInfo struct {
	Mint     string
	Symbol   string
	Decimals uint8
}

func GetTokenMint(token string) string {
	if t, ok := Tokens[token]; ok {
		return t.Mint
	}
	return Tokens["usdc"].Mint
}

func GetTokenDecimals(token string) uint8 {
	if t, ok := Tokens[token]; ok {
		return t.Decimals
	}
	return 6
}

func GetTokenSymbol(token string) string {
	if t, ok := Tokens[token]; ok {
		return t.Symbol
	}
	return "USDC"
}
