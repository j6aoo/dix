# DIX - Decentralized Instant eXchange

Sistema de pagamentos peer-to-peer em Solana com usernames legiveis. Tipo um PIX, so que sem banco, sem governo, sem KYC.

```
dix register joao
dix pay usdc joao 100
dix pool create vaquinha usdc 50
```

---

## Por que DIX existe

Vou ser direto: o sistema financeiro tradicional e uma merda pra transferencias. PIX melhorou muito no Brasil, mas ainda depende de bancos, CPF, e um monte de intermediarios que podem bloquear sua grana a qualquer momento. Crypto resolveu parte do problema, mas criou outro: ninguem quer decorar ou compartilhar um endereco tipo `7xKXtg2CW87d97TXJSDpbD5jBkheTqA83TZRuJosgAsU`.

DIX e uma tentativa de pegar o melhor dos dois mundos. Voce registra um username simples na blockchain, e qualquer pessoa pode te mandar USDC usando so esse nome. Sem intermediarios, sem censura, sem banco decidindo se voce pode ou nao receber dinheiro.

A ideia nao e nova. O proprio PIX usa chaves (CPF, email, telefone) pra resolver pra uma conta bancaria. A diferenca e que aqui tudo roda on-chain, a chave privada fica com voce, e ninguem pode te impedir de transacionar.

O projeto nasceu de uma frustacao real: queria mandar dinheiro pra um amigo no exterior sem pagar taxa de 5% e esperar 3 dias. Com DIX, a transacao confirma em 400ms e custa menos de 1 centavo.


## Arquitetura geral

O sistema tem duas partes principais:

1. Um programa Anchor rodando na Solana que guarda o mapeamento username -> endereco
2. Uma CLI em Go que faz toda a interacao com a rede

Nao tem backend, nao tem API, nao tem servidor. Tudo roda local na maquina do usuario. Os unicos dados que saem sao as transacoes enviadas diretamente pro RPC da Solana.

```
Usuario -> CLI (Go) -> Solana RPC -> Programa Anchor
                |
                v
          SQLite local (historico)
```

A escolha de Go foi pragmatica. Compila pra um binario unico sem dependencias, roda em qualquer OS, e o pessoal de infra ja conhece. Considerei Rust, mas a curva de aprendizado ia afastar contribuidores. Python seria mais facil de escrever, mas distribuir e um inferno.

O SQLite local existe so pra manter um historico de transacoes. Nao e essencial pro funcionamento - voce pode deletar o arquivo e continuar usando normalmente. Mas e util pra saber o que ja foi enviado sem ter que consultar a blockchain toda hora.


## O programa Solana

O contrato (programa, no jargao Solana) e bem simples. Basicamente faz uma coisa: mapeia strings pra enderecos.

```rust
#[account]
pub struct Alias {
    pub username: String,  // max 20 caracteres
    pub owner: Pubkey,     // endereco da carteira
    pub created_at: i64,   // timestamp
}
```

Cada username registrado vira uma PDA (Program Derived Address) com seeds `["alias", username]`. Isso significa que dado um username, qualquer um consegue calcular deterministicamente onde esta o account com os dados.

A vantagem de usar PDA e que nao precisa de um indice centralizado. Quer saber o endereco do "joao"? Deriva a PDA, busca o account, le o campo `owner`. Pronto.

```rust
pub fn register(ctx: Context<Register>, username: String) -> Result<()> {
    require!(username.len() >= 3 && username.len() <= 20, DixError::InvalidUsername);

    let alias = &mut ctx.accounts.alias;
    alias.username = username;
    alias.owner = ctx.accounts.user.key();
    alias.created_at = Clock::get()?.unix_timestamp;

    Ok(())
}
```

Registrar um username custa o rent do account (cerca de 0.001 SOL). Uma vez pago, o username e seu pra sempre. Nao tem renovacao, nao tem taxa recorrente.

Optei por nao cobrar taxa de protocolo no registro. A maioria dos projetos cobra uma taxa "pra sustentar o desenvolvimento", mas na pratica isso so cria incentivo pra especulacao de usernames. Prefiro manter simples.


## A CLI

O ponto de entrada e o `cmd/dix/main.go`. Usa Cobra pra parsing de comandos, mas de forma bem minimalista. Nada de subcomandos aninhados ou flags mirabolantes.

```
dix init                       # cria carteira
dix recover                    # recupera de mnemonic
dix register <user>            # registra username
dix pay <token> <to> <amount>  # envia tokens
dix balance                    # mostra saldo
dix ledger                     # historico local
dix tokens                     # lista tokens suportados
dix pool create/join/pay/...   # consorcios
```

Tokens suportados: `usdc`, `usdt`, `btc` (wBTC), `ltc` (wLTC)

Cada comando faz uma coisa so. Se der erro, printa o erro e sai com codigo 1. Nada de logs estruturados, nada de telemetria, nada de "voce quis dizer X?".

Os arquivos ficam em `~/.dix/`:

```
~/.dix/
├── keypair.json   # chave privada (criptografada com AES-256-GCM)
└── ledger.db      # historico SQLite
```

A chave privada nunca e salva em texto claro. Quando voce roda `dix init`, o sistema pede uma senha e usa ela pra derivar uma chave AES. O keypair e criptografado antes de ir pro disco.

Isso nao e seguranca perfeita. Se alguem tem acesso ao seu filesystem E sabe sua senha, perdeu. Mas protege contra o caso mais comum: alguem copia o arquivo sem saber a senha.


## Geracao de chaves

A carteira usa Ed25519, que e o padrao da Solana. O fluxo e:

1. Gera 256 bits de entropia
2. Converte pra mnemonic BIP39 (24 palavras)
3. Deriva seed de 64 bytes do mnemonic
4. Usa os primeiros 32 bytes como chave privada Ed25519
5. Deriva chave publica

```go
func Generate() (mnemonic string, wallet Wallet, err error) {
    entropy, _ := bip39.NewEntropy(256)
    mnemonic, _ = bip39.NewMnemonic(entropy)
    seed := bip39.NewSeed(mnemonic, "")
    
    priv := ed25519.NewKeyFromSeed(seed[:32])
    pub := priv.Public().(ed25519.PublicKey)
    
    // Formato Solana: 32 bytes priv + 32 bytes pub
    secret := make([]byte, 64)
    copy(secret[:32], seed[:32])
    copy(secret[32:], pub)
    
    return mnemonic, Wallet{Pubkey: base58.Encode(pub), Secret: secret}, nil
}
```

O formato do keypair (64 bytes = priv + pub concatenados) e compativel com outras ferramentas Solana. Voce pode exportar e usar no Phantom ou qualquer outra wallet.

Por que BIP39 ao inves de gerar bytes aleatorios direto? Porque humanos conseguem anotar 24 palavras num papel. Ninguem vai anotar 64 bytes em hex sem errar.


## Resolucao de usernames

Quando voce roda `dix pay joao 100`, o sistema precisa descobrir o endereco do `joao`. O fluxo e:

1. Verifica se `joao` parece um username (3-20 chars, lowercase, alphanumerico)
2. Se nao, assume que e um endereco Solana direto
3. Se sim, busca no cache local (SQLite)
4. Se nao ta no cache, deriva a PDA e busca on-chain
5. Guarda no cache pra proxima vez

```go
func Resolve(db *sql.DB, username string, programID, rpcURL string) (solana.PublicKey, error) {
    username = strings.ToLower(username)
    
    // Cache local primeiro
    cached, err := Getalias(db, username)
    if err == nil && cached != "" {
        return solana.PublicKeyFromBase58(cached)
    }
    
    // Busca on-chain
    program := solana.MustPublicKeyFromBase58(programID)
    pda, _, _ := solana.FindProgramAddress(
        [][]byte{[]byte("alias"), []byte(username)},
        program,
    )
    
    client := rpc.New(rpcURL)
    acct, err := client.GetAccountInfo(context.Background(), pda)
    if err != nil || acct.Value == nil {
        return solana.PublicKey{}, fmt.Errorf("username not found: %s", username)
    }
    
    // Parse: skip 8 bytes discriminator, read 32 bytes pubkey
    data := acct.Value.Data.GetBinary()
    owner := solana.PublicKeyFromBytes(data[8:40])
    
    // Guarda no cache
    Savealias(db, username, owner.String())
    
    return owner, nil
}
```

O cache evita bater na rede toda vez. Mas tem um tradeoff: se o dono do username mudar o endereco, voce vai mandar pro endereco antigo ate o cache expirar. Por ora nao implementei expiracao - assumo que username nao muda de dono frequentemente.


## Fluxo de pagamento

O `pay.go` contem toda a logica de envio. O fluxo e:

1. Cria um Intent (estrutura local que representa a intencao de pagar)
2. Verifica idempotencia (ja tentou esse pagamento antes?)
3. Resolve o destinatario (username -> endereco)
4. Valida (amount > 0, destinatario valido, etc)
5. Monta a transacao SPL Token Transfer
6. Envia e aguarda confirmacao
7. Atualiza status no SQLite

```go
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
    
    // Idempotencia
    existing, err := Load(db, i.ID)
    if err == nil {
        fmt.Printf("intent %s exists (status: %s)\n", existing.ID[:8], existing.Status)
        return nil
    }
    
    Save(db, i)
    
    // Resolve destinatario
    var toPubkey solana.PublicKey
    if IsUsername(to) {
        toPubkey, err = Resolve(db, to, programID, rpcURL)
        // ...
    }
    
    // Envia
    sig, err := Send(from, toPubkey, amount, keypair, rpcURL)
    // ...
    
    // Aguarda confirmacao
    Confirm(sig, rpcURL, 30*time.Second)
    
    i.Status = "done"
    Save(db, i)
    
    return nil
}
```

O ID do intent e um hash de (from, to, amount, timestamp). Isso garante que se voce rodar o mesmo comando duas vezes por acidente, a segunda execucao detecta e nao reenvia.

A confirmacao usa polling simples: a cada 200ms verifica se a transacao foi confirmada. Quando o status muda pra `confirmed` ou `finalized`, retorna. Timeout de 30 segundos.


## Transferencia SPL

O `sol.go` lida com a parte Solana. Pra transferir USDC, precisa:

1. Descobrir as Associated Token Accounts (ATA) do sender e receiver
2. Montar a instrucao de transfer
3. Pegar blockhash recente
4. Montar e assinar a transacao
5. Enviar

```go
func Send(from, to solana.PublicKey, amount uint64, keypair solana.PrivateKey, rpcURL string) (string, error) {
    client := rpc.New(rpcURL)
    
    fromATA, _, _ := solana.FindAssociatedTokenAddress(from, solana.MustPublicKeyFromBase58(USDCMint))
    toATA, _, _ := solana.FindAssociatedTokenAddress(to, solana.MustPublicKeyFromBase58(USDCMint))
    
    transferIx := token.NewTransferInstruction(
        amount,
        fromATA,
        toATA,
        from,
        []solana.PublicKey{},
    ).Build()
    
    recent, _ := client.GetLatestBlockhash(context.Background(), rpc.CommitmentFinalized)
    
    tx, _ := solana.NewTransaction(
        []solana.Instruction{transferIx},
        recent.Value.Blockhash,
        solana.TransactionPayer(from),
    )
    
    tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
        if key.Equals(from) {
            return &keypair
        }
        return nil
    })
    
    sig, _ := client.SendTransaction(context.Background(), tx)
    return sig.String(), nil
}
```

Um problema que nao tratei ainda: se o receiver nao tem ATA criada, a transacao falha. O certo seria incluir uma instrucao de createAssociatedTokenAccount antes do transfer. Ta na lista de coisas pra fazer.


## Banco de dados local

O SQLite guarda duas tabelas:

```sql
CREATE TABLE intents (
    id TEXT PRIMARY KEY,
    from_pubkey TEXT,
    to_pubkey TEXT,
    to_resolved TEXT,
    amount INTEGER,
    signature TEXT,
    time INTEGER,
    status TEXT
);

CREATE TABLE aliases (
    username TEXT PRIMARY KEY,
    pubkey TEXT
);
```

A tabela `intents` e o ledger local. Cada pagamento vira uma linha. O status vai de `pending` -> `sent` -> `done` (ou `fail`).

A tabela `aliases` e cache de resolucao. Evita bater na rede pra resolver usernames que ja foram resolvidos antes.

Por que SQLite e nao um arquivo JSON ou algo mais simples? Porque SQL resolve queries que eu nao quero implementar na mao. "Me da os ultimos 20 pagamentos ordenados por data" e uma linha de SQL. Fazer isso com arquivo JSON seria um saco.

Por que nao Postgres ou MySQL? Porque exigiria o usuario instalar e configurar um banco. SQLite e um arquivo. Copia o arquivo, ta copiado o banco.


## Pools (Consorcios)

O sistema de pools e um consorcio P2P entre amigos. Funciona assim:

1. Alguem cria um pool: `dix pool create vaquinha usdc 100`
2. Amigos entram: `dix pool join abc123def456`
3. O criador inicia: `dix pool start abc123def456`
4. Todo mes cada membro paga: `dix pool pay abc123def456`
5. O ganhador da rodada recebe tudo direto na carteira
6. Quando todos pagaram, o ganhador confirma: `dix pool claim abc123def456`
7. Proximo round comeca

A ordem de quem ganha e definida pela ordem de entrada. O primeiro a entrar ganha a primeira rodada, o segundo ganha a segunda, etc.

```
Pool "vaquinha" (5 membros, 100 USDC/round):

Round 1: todos pagam 100 USDC → joao recebe 500 USDC
Round 2: todos pagam 100 USDC → maria recebe 500 USDC
Round 3: todos pagam 100 USDC → pedro recebe 500 USDC
...
```

Comandos:

```
dix pool create <name> <token> <amount>  # cria pool
dix pool join <id>                        # entra num pool
dix pool start <id>                       # inicia (fecha registro)
dix pool pay <id>                         # paga sua parte
dix pool claim <id>                       # confirma recebimento
dix pool status <id>                      # mostra estado atual
dix pool list                             # lista seus pools
```

Por que funciona sem smart contract de escrow? Porque os pagamentos vao direto pro ganhador da rodada. Nao tem custodia. Quem nao pagar simplesmente fica marcado como inadimplente e os outros veem.

A garantia aqui e social, nao tecnica. Por isso e pra fazer com amigos, nao com estranhos. Se alguem furar, voce sabe quem foi.


## Tratamento de erros

A filosofia aqui e: se deu erro, printa e sai. Nada de tentar recuperar, nada de retry automatico, nada de "vou tentar de novo em 5 segundos".

```go
if err != nil {
    i.Status = "fail"
    Save(db, i)
    return fmt.Errorf("send: %w", err)
}
```

Isso parece tosco, mas e intencional. Pagamentos sao operacoes sensiveis. Se algo deu errado, eu quero que o usuario saiba e decida o que fazer. Retry automatico pode causar pagamento duplicado (mesmo com idempotencia, edge cases existem).

O unico lugar com retry e a confirmacao: fica em loop ate confirmar ou dar timeout. Mas o envio em si nao tem retry.


## Custos

Tem dois custos no sistema:

1. Registrar username: ~0.001 SOL (rent do account)
2. Transferir USDC: ~0.000005 SOL (fee da transacao)

Na cotacao atual, isso da uns 20 centavos pra registrar e menos de 1 centavo por transfer. O protocolo nao cobra nada alem disso.

Considerei adicionar uma taxa de protocolo (tipo 0.1% do valor transferido) pra sustentar desenvolvimento. Decidi nao fazer por alguns motivos:

- Complica o codigo (precisa de conta pra receber, logica de split, etc)
- Cria incentivo pra fork sem taxa
- O sistema e simples demais pra justificar taxa

Se precisar de grana pra manter, vou buscar grants ou fazer servicos em cima. O protocolo base fica gratuito.


## Seguranca

Alguns pontos importantes:

**Chave privada**: Fica no disco criptografada. Se alguem conseguir o arquivo E a senha, tem acesso total. Nao tem 2FA, nao tem hardware wallet, nao tem MPC. E um arquivo com senha. Simples assim.

**Username squatting**: Qualquer um pode registrar qualquer username. Nao tem verificacao de identidade. Se voce quiser "satoshi", e so registrar primeiro. Isso e uma feature, nao um bug - nao quero ser arbitro de quem merece qual nome.

**Transacoes irreversiveis**: Uma vez confirmada, nao tem como reverter. Nao tem chargeback, nao tem disputa, nao tem suporte. Confere o destinatario antes de enviar.

**Phishing**: Alguem pode registrar "joao" parecido com outro "joao" (tipo com caractere unicode parecido). A validacao atual so aceita a-z e 0-9, mas isso pode mudar. Sempre confira o endereco resolvido antes de mandar valores grandes.


## Limitacoes conhecidas

Vou ser honesto sobre o que nao funciona ou nao existe ainda:

**Solana only**: Nao tem bridge, nao tem cross-chain, nao tem nada. So Solana mainnet (ou devnet pra teste).

**CLI only**: Sem app mobile, sem interface web, sem API REST. Voce precisa de terminal. Isso limita muito o publico, mas era o que eu conseguia fazer rapido.

**Sem ATA auto-create**: Se o destinatario nunca recebeu o token antes, a transacao falha. Deveria criar a ATA automaticamente.

**Sem QR Code**: PIX tem QR Code pra facilitar. DIX nao tem nada disso ainda.

**Username permanente**: Uma vez registrado, nao da pra deletar ou transferir. O owner pode ser atualizado, mas o username em si fica la pra sempre.


## O que nao vai ter

Algumas coisas eu deliberadamente nao quero implementar:

**Clean Architecture**: O codigo e procedural. Funcoes que fazem coisas. Nao tem interfaces, nao tem dependency injection, nao tem repository pattern. Isso deixa o codigo mais facil de ler e mais dificil de abstrair demais.

**Microservicos**: Tudo roda num binario. Nao tem fila de mensagens, nao tem event sourcing, nao tem CQRS. Um binario, uma responsabilidade.

**Telemetria**: Nao mando dados pra lugar nenhum. Nem crash reports, nem analytics, nem nada. Sua privacidade > minha conveniencia.

**Accounts/Login**: Nao existe o conceito de "conta no DIX". Sua carteira e sua identidade. Perdeu a chave, perdeu acesso. Simples.


## Contribuindo

O projeto e MIT, contribuicoes sao bem vindas. Algumas guidelines:

1. Mantenha simples. Se a solucao precisa de 3 novos arquivos e uma interface, provavelmente ta errada.
2. Testes sao opcionais. Prefiro codigo obvio que nao precisa de teste do que codigo complexo coberto de testes.
3. Nada de linters dogmaticos. Se o codigo funciona e e legivel, ta bom.
4. Commits pequenos. Um commit = uma mudanca logica.

Pra rodar local:

```bash
git clone https://github.com/your-org/dix
cd dix
go build -o dix ./cmd/dix
./dix --rpc https://api.devnet.solana.com init
```

Pra pegar SOL de teste:

```bash
solana airdrop 1 <seu-pubkey> --url devnet
```

Pra deploy do programa:

```bash
cd program
anchor build
anchor deploy --provider.cluster devnet
# copia o program ID pro types.go
```


## Filosofia

DIX nao e pra todo mundo. E pra quem:

- Prefere controle a conveniencia
- Aceita riscos em troca de liberdade
- Sabe usar terminal
- Entende que "sem intermediario" significa "sem suporte"

Se voce quer algo facil, com app bonito, suporte 24h e protecao contra erro humano, use PIX. Esta tudo bem. Ferramentas diferentes pra pessoas diferentes.

Mas se voce quer mandar dinheiro pra qualquer lugar do mundo, sem pedir permissao, sem taxa abusiva, sem risco de ter conta bloqueada... DIX ta aqui.

---

## Sobre o desenvolvimento

Vou ser transparente: esse codigo foi escrito com assistencia de IA. Mas tem uma diferenca importante entre "vibecoding" e usar IA como ferramenta.

Vibecoding e quando voce joga um prompt, aceita o que vier, e reza pra funcionar. Nao foi isso que aconteceu aqui.

Cada funcao foi revisada. Cada decisao de arquitetura foi questionada. Os tradeoffs foram pensados. O codigo foi testado. Quando a IA sugeria algo que nao fazia sentido pro contexto, era descartado. 

A IA acelerou o desenvolvimento. Escreveu boilerplate, sugeriu estruturas, ajudou com sintaxe de bibliotecas que eu nao conhecia de cor. Mas as decisoes de design, por que Go ao inves de Rust, por que SQLite ao inves de arquivo JSON, por que nao cobrar taxa de protocolo, essas foram humanas.

Se voce ta lendo isso pensando em contribuir: o codigo e legivel, os nomes fazem sentido, e a arquitetura e simples de propósito. Nao foi acidente.

---

MIT License - faz o que quiser com o codigo.
