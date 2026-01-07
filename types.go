package dix

type Intent struct {
	ID        string
	From      string 
	To        string 
	ToResolved string 
	Amount    uint64 
	Signature string 
	Time      int64
	Status    string 
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
	
	USDCMint = "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v"
	
	RegistryProgram = ""
)
