package bid

import (
	"context"
	"fmt"
	"fullcycle-auction_go/internal/entity/auction_entity"
	"fullcycle-auction_go/internal/entity/bid_entity"
	"fullcycle-auction_go/internal/infra/database/auction"
	"fullcycle-auction_go/internal/usecase/bid_usecase"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func startMongoContainer(ctx context.Context, t *testing.T) (testcontainers.Container, string) {
	t.Helper()

	req := testcontainers.ContainerRequest{
		Image:        "mongo:6",
		ExposedPorts: []string{"27017/tcp"},
		WaitingFor:   wait.ForLog("Waiting for connections").WithStartupTimeout(30 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("failed to start mongo container: %v", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("failed to get container host: %v", err)
	}

	port, err := container.MappedPort(ctx, "27017")
	if err != nil {
		t.Fatalf("failed to get container port: %v", err)
	}

	return container, fmt.Sprintf("mongodb://%s:%s", host, port.Port())
}

func newTestRepos(ctx context.Context, t *testing.T, uri string) (*auction.AuctionRepository, *BidRepository) {
	t.Helper()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatalf("failed to connect to mongo: %v", err)
	}
	t.Cleanup(func() { client.Disconnect(ctx) })

	db := client.Database("auctions_test")
	auctionRepo := auction.NewAuctionRepository(db)
	bidRepo := NewBidRepository(db, auctionRepo)
	return auctionRepo, bidRepo
}

func createTestAuction(ctx context.Context, t *testing.T, auctionRepo *auction.AuctionRepository) *auction_entity.Auction {
	t.Helper()

	a, err := auction_entity.CreateAuction(
		"Fender Telecaster 1952",
		"Musical Instruments",
		"Original 1952 Fender Telecaster in collector condition",
		auction_entity.Used,
	)
	require.Nil(t, err)
	require.Nil(t, auctionRepo.CreateAuction(ctx, a))
	return a
}

// TestCreateBidAndFindByAuctionId verifies that bids are persisted and retrievable.
// This test would have caught the BSON field name bug ("auctionId" vs "auction_id").
func TestCreateBidAndFindByAuctionId(t *testing.T) {
	ctx := context.Background()
	t.Setenv("AUCTION_INTERVAL", "60s")

	container, uri := startMongoContainer(ctx, t)
	defer container.Terminate(ctx)

	auctionRepo, bidRepo := newTestRepos(ctx, t, uri)
	a := createTestAuction(ctx, t, auctionRepo)

	userId := uuid.New().String()
	bid1, err := bid_entity.CreateBid(userId, a.Id, 500.0)
	require.Nil(t, err)

	bid2, err := bid_entity.CreateBid(userId, a.Id, 1200.0)
	require.Nil(t, err)

	internalErr := bidRepo.CreateBid(ctx, []bid_entity.Bid{*bid1, *bid2})
	assert.Nil(t, internalErr)

	bids, internalErr := bidRepo.FindBidByAuctionId(ctx, a.Id)
	assert.Nil(t, internalErr)
	assert.Len(t, bids, 2, "both bids must be persisted and returned")

	amounts := make([]float64, 0, len(bids))
	for _, b := range bids {
		amounts = append(amounts, b.Amount)
		assert.Equal(t, a.Id, b.AuctionId, "auction_id must match")
		assert.Equal(t, userId, b.UserId, "user_id must match")
	}
	assert.ElementsMatch(t, []float64{500.0, 1200.0}, amounts)
}

// TestFindWinningBidByAuctionId verifies that the highest bid is returned as winner.
func TestFindWinningBidByAuctionId(t *testing.T) {
	ctx := context.Background()
	t.Setenv("AUCTION_INTERVAL", "60s")

	container, uri := startMongoContainer(ctx, t)
	defer container.Terminate(ctx)

	auctionRepo, bidRepo := newTestRepos(ctx, t, uri)
	a := createTestAuction(ctx, t, auctionRepo)

	userId := uuid.New().String()
	bids := []bid_entity.Bid{}
	for _, amount := range []float64{300.0, 750.0, 1500.0, 900.0} {
		b, err := bid_entity.CreateBid(userId, a.Id, amount)
		require.Nil(t, err)
		bids = append(bids, *b)
	}

	internalErr := bidRepo.CreateBid(ctx, bids)
	assert.Nil(t, internalErr)

	winner, internalErr := bidRepo.FindWinningBidByAuctionId(ctx, a.Id)
	assert.Nil(t, internalErr)
	require.NotNil(t, winner)
	assert.Equal(t, 1500.0, winner.Amount, "winner must be the highest bid")
	assert.Equal(t, a.Id, winner.AuctionId)
}

// TestBidRejectedOnExpiredAuction verifies that the repository layer silently
// discards bids after the auction interval expires (no error, no persistence).
// The use case layer is responsible for returning a proper error — see TestBidUseCaseRejectsClosedAuction.
func TestBidRejectedOnExpiredAuction(t *testing.T) {
	ctx := context.Background()
	t.Setenv("AUCTION_INTERVAL", "1s")

	container, uri := startMongoContainer(ctx, t)
	defer container.Terminate(ctx)

	auctionRepo, bidRepo := newTestRepos(ctx, t, uri)
	a := createTestAuction(ctx, t, auctionRepo)

	// Wait for the auction to expire
	time.Sleep(2 * time.Second)

	userId := uuid.New().String()
	lateBid, err := bid_entity.CreateBid(userId, a.Id, 9999.0)
	require.Nil(t, err)

	internalErr := bidRepo.CreateBid(ctx, []bid_entity.Bid{*lateBid})
	assert.Nil(t, internalErr, "repository layer must not return error for expired auctions")

	// The bid must NOT have been persisted
	bids, internalErr := bidRepo.FindBidByAuctionId(ctx, a.Id)
	assert.Nil(t, internalErr)
	assert.Empty(t, bids, "no bids should be persisted for an expired auction")
}

// TestBidUseCaseRejectsClosedAuction verifies that the use case returns a conflict
// error (mapped to HTTP 422) when a bid is placed on a closed auction.
func TestBidUseCaseRejectsClosedAuction(t *testing.T) {
	ctx := context.Background()
	t.Setenv("AUCTION_INTERVAL", "1s")
	t.Setenv("BATCH_INSERT_INTERVAL", "10s")
	t.Setenv("MAX_BATCH_SIZE", "10")

	container, uri := startMongoContainer(ctx, t)
	defer container.Terminate(ctx)

	auctionRepo, bidRepo := newTestRepos(ctx, t, uri)
	a := createTestAuction(ctx, t, auctionRepo)

	// Wait for the goroutine to close the auction in MongoDB
	time.Sleep(2 * time.Second)

	uc := bid_usecase.NewBidUseCase(bidRepo, auctionRepo)

	internalErr := uc.CreateBid(ctx, bid_usecase.BidInputDTO{
		UserId:    uuid.New().String(),
		AuctionId: a.Id,
		Amount:    9999.0,
	})

	require.NotNil(t, internalErr, "use case must return an error for a closed auction")
	assert.Equal(t, "conflict", internalErr.Err, "error type must be 'conflict' (maps to HTTP 422)")
}

// TestBidValidation verifies that invalid bid inputs are rejected at the entity level.
func TestBidValidation(t *testing.T) {
	validAuctionId := uuid.New().String()
	validUserId := uuid.New().String()

	_, err := bid_entity.CreateBid("not-a-uuid", validAuctionId, 100.0)
	assert.NotNil(t, err, "invalid userId UUID must be rejected")

	_, err = bid_entity.CreateBid(validUserId, "not-a-uuid", 100.0)
	assert.NotNil(t, err, "invalid auctionId UUID must be rejected")

	_, err = bid_entity.CreateBid(validUserId, validAuctionId, -50.0)
	assert.NotNil(t, err, "negative amount must be rejected")

	_, err = bid_entity.CreateBid(validUserId, validAuctionId, 0)
	assert.NotNil(t, err, "zero amount must be rejected")

	validBid, err := bid_entity.CreateBid(validUserId, validAuctionId, 100.0)
	assert.Nil(t, err, "valid bid must not return error")
	assert.NotNil(t, validBid)
}
