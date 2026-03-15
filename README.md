# Auction System — FullCycle Labs

REST API for managing auctions with automatic closing, built in Go with Hexagonal Architecture.

## Features

- Create auctions and place bids
- Auctions close automatically after a configurable duration (goroutine-based)
- Winner query returns the highest bid for a closed auction
- Bid batching for reduced MongoDB write pressure

## Requirements

- Docker and Docker Compose

## Running the project

```bash
docker-compose up --build
```

The API will be available at `http://localhost:8080`.

## Environment variables

All variables are configured in `cmd/auction/.env` and injected via Docker Compose.

| Variable | Description | Default |
|---|---|---|
| `AUCTION_INTERVAL` | Duration after which an auction is automatically closed | `20s` |
| `BATCH_INSERT_INTERVAL` | Interval for flushing the in-memory bid batch to MongoDB | `20s` |
| `MAX_BATCH_SIZE` | Number of bids that triggers an immediate batch flush | `4` |
| `MONGODB_URL` | MongoDB connection string | `mongodb://admin:admin@mongodb:27017/auctions?authSource=admin` |
| `MONGODB_DB` | MongoDB database name | `auctions` |

### Configuring auction duration

Edit `cmd/auction/.env` and set `AUCTION_INTERVAL` to any valid Go duration string:

```env
AUCTION_INTERVAL=5m   # 5 minutes
AUCTION_INTERVAL=30s  # 30 seconds
AUCTION_INTERVAL=1h   # 1 hour
```

Rebuild the container after changing the value:

```bash
docker-compose up --build
```

## API endpoints

| Method | Path | Description |
|---|---|---|
| `POST` | `/auction` | Create a new auction |
| `GET` | `/auction` | List auctions (optional filters: `status`, `category`, `productName`) |
| `GET` | `/auction/:auctionId` | Get auction by ID |
| `GET` | `/auction/winner/:auctionId` | Get the winning bid for an auction |
| `POST` | `/bid` | Place a bid |
| `GET` | `/bid/:auctionId` | List bids for an auction |
| `GET` | `/user/:userId` | Get user by ID |

### Example: Create auction

```bash
curl -X POST http://localhost:8080/auction \
  -H "Content-Type: application/json" \
  -d '{
    "product_name": "Vintage Guitar",
    "category": "Music Instruments",
    "description": "A rare vintage guitar in excellent condition",
    "condition": 2
  }'
```

Condition values: `1` = New, `2` = Used, `3` = Refurbished

### Example: Place a bid

```bash
curl -X POST http://localhost:8080/bid \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "<valid-user-uuid>",
    "auction_id": "<auction-uuid>",
    "amount": 1500.00
  }'
```

## Running tests

The integration tests use [Testcontainers](https://testcontainers.com/) and require Docker running locally.

```bash
go test ./internal/infra/database/auction/... -v -timeout 300s
go test ./internal/infra/database/bid/... -v -timeout 300s
```

## Architecture

```
cmd/auction/                       → entry point, dependency wiring
configuration/                     → MongoDB, logger, error helpers
internal/entity/                   → domain entities and repository interfaces
internal/usecase/                  → application use cases
internal/infra/database/           → MongoDB repository implementations
internal/infra/api/web/controller/ → HTTP controllers (Gin)
```
