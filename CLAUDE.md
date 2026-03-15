# CLAUDE.md

## Papel e contexto

Você está implementando um sistema de leilões HTTP em Go. Siga estas instruções à risca em toda interação com este projeto.

---

## Regras de código — NUNCA faça

- Nunca use `panic` em código de aplicação
- Nunca ignore erros — trate todos explicitamente
- Nunca hardcode valores de configuração
- Nunca adicione comentários que apenas repetem o que o código já diz
- Nunca adicione dependências externas sem necessidade clara
- Nunca escreva logs desnecessários — apenas o que agrega valor em produção
- Nunca use `IRepository`, `RepositoryInterface` ou sufixos redundantes em interfaces
- Nunca implemente lógica de negócio dentro dos controllers (`infra/api/`)
- Nunca implemente lógica HTTP dentro de `entity/` ou `usecase/`
- Nunca use `context.Background()` dentro de handlers — sempre propague o context da request

---

## Convenções obrigatórias

### Nomenclatura
- Interfaces: nome descritivo simples — `AuctionRepository`, não `IAuctionRepository`
- Implementações: nome concreto + tipo — `AuctionRepositoryMongo`
- Construtores: sempre `New{Type}` — `NewAuctionUseCase`, `NewBidRepository`
- Métodos: verbos claros — `CreateAuction`, `FindWinningBid`, `PlaceBid`

### Go idiomático
- Use `context.Context` em todas as operações de I/O
- Prefira composição
- Interfaces pequenas — no máximo 3–4 métodos
- Nomes que dispensam comentários
- Siga Effective Go, Go Code Review Comments e Google Go Style Guide

### Arquitetura Hexagonal — Ports & Adapters

```
internal/entity/                   → CORE: domínio puro, entidades e interfaces (ports)
  ├── auction_entity/              → entidade Auction + AuctionRepositoryInterface
  ├── bid_entity/                  → entidade Bid + BidRepositoryInterface
  └── user_entity/                 → entidade User + UserRepositoryInterface

internal/usecase/                  → APPLICATION: casos de uso, orquestram o domínio
  ├── auction_usecase/
  ├── bid_usecase/
  └── user_usecase/

internal/infra/database/           → SECONDARY ADAPTER: implementações MongoDB
  ├── auction/
  ├── bid/
  └── user/

internal/infra/api/web/controller/ → PRIMARY ADAPTER: controllers Gin (HTTP ↔ use case)
  ├── auction_controller/
  ├── bid_controller/
  └── user_controller/

configuration/                     → configuração, logger, error helpers
cmd/auction/                       → entry point, wiring de dependências, rotas
```

Respeite os limites de cada camada:
- `entity/` não importa nada de `infra/` nem pacotes HTTP
- `usecase/` não importa nada de `infra/`
- `infra/api/` não contém regra de negócio — apenas tradução HTTP ↔ use case
- `infra/database/` implementa as interfaces definidas em `entity/`

---

## Checklist antes de cada commit

- [ ] Todos os erros estão sendo tratados
- [ ] Nenhum valor hardcoded — tudo vem de variáveis de ambiente
- [ ] `context.Context` propagado da request até o repositório em todas as operações de I/O
- [ ] Limites de camada respeitados (entity sem HTTP, controller sem regra de negócio)
- [ ] Testes adicionados ou atualizados para a feature
- [ ] `go vet ./...` e `go build ./...` sem erros
- [ ] `go mod tidy` rodado

---

## Git workflow

### Branches
- Crie uma branch por feature a partir da `main`
- Nomenclatura: `feat/`, `fix/`, `test/`, `chore/`, `docs/`
- Após aprovação do usuário, faça merge na `main`
- A próxima branch sempre parte da `main` atualizada

### Fluxo
1. `git checkout main`
2. `git checkout -b feat/nome-da-feature`
3. Implemente em commits atômicos
4. Adicione ou atualize os testes da feature antes de commitar
5. Apresente os arquivos ao usuário para aprovação
6. `git add <arquivos específicos>` — nunca `git add .`
7. Commit após aprovação explícita do usuário
8. `git push -u origin feat/nome-da-feature` — suba a branch para o remoto após o commit
9. Merge na `main`
10. `git push origin main` — suba a `main` atualizada para o remoto após o merge

### Commits
- Mensagens em inglês, Conventional Commits
- `feat:` `fix:` `test:` `chore:` `docs:`
- Um commit = uma mudança lógica
- Nunca mencionar Claude ou IA na mensagem

---

## Notas críticas de implementação

### Context propagation
Sempre use `c.Request.Context()` nos controllers e propague até o repositório. Nunca substitua por `context.Background()`.

### Concorrência no BidUseCase
O batch de bids usa um channel + goroutine. Qualquer modificação nessa área deve:
- Proteger estruturas compartilhadas com `sync.Mutex`
- Garantir que o batch seja drenado no shutdown (graceful shutdown)
- Nunca assumir que o resultado do `CreateBid` foi persistido imediatamente (é assíncrono)

### Atomicidade em operações críticas
Verificações de status + escrita devem ser atômicas quando possível. Evite race conditions entre leitura de cache e inserção no MongoDB.

### Nomes de campos BSON
Use sempre a tag `bson:"nome_do_campo"` explicitamente nas structs Mongo. O mapeamento automático pode gerar inconsistências silenciosas entre filtros e documentos armazenados.

---

## Dependências do projeto

```
github.com/gin-gonic/gin                  # HTTP framework
go.mongodb.org/mongo-driver               # MongoDB client
github.com/google/uuid                    # UUID generation & validation
github.com/joho/godotenv                  # carrega .env
github.com/go-playground/validator/v10    # validação de input
go.uber.org/zap                           # structured logging
```

Adicione dependências com `go get`, finalize com `go mod tidy`.
