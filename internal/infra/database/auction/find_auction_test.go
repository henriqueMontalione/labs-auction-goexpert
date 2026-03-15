package auction

import (
	"context"
	"fullcycle-auction_go/internal/entity/auction_entity"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindAuctionById(t *testing.T) {
	ctx := context.Background()
	container, uri := startMongoContainer(ctx, t)
	defer container.Terminate(ctx)

	repo := newTestRepo(ctx, t, uri)

	auction, err := auction_entity.CreateAuction(
		"Fender Stratocaster",
		"Musical Instruments",
		"Original 1965 Fender Stratocaster in excellent condition",
		auction_entity.Used,
	)
	require.Nil(t, err)
	require.Nil(t, repo.CreateAuction(ctx, auction))

	found, internalErr := repo.FindAuctionById(ctx, auction.Id)
	assert.Nil(t, internalErr)
	assert.Equal(t, auction.Id, found.Id)
	assert.Equal(t, auction.ProductName, found.ProductName)
	assert.Equal(t, auction.Category, found.Category)
	assert.Equal(t, auction.Status, found.Status)
	assert.Equal(t, auction_entity.Active, found.Status)
}

func TestFindAuctions_FilterByStatus(t *testing.T) {
	ctx := context.Background()
	container, uri := startMongoContainer(ctx, t)
	defer container.Terminate(ctx)

	t.Setenv("AUCTION_INTERVAL", "1s")
	repo := newTestRepo(ctx, t, uri)

	active, err := auction_entity.CreateAuction(
		"Active Guitar",
		"Musical Instruments",
		"A guitar that is still active and open for bids",
		auction_entity.New,
	)
	require.Nil(t, err)
	require.Nil(t, repo.CreateAuction(ctx, active))

	closing, err := auction_entity.CreateAuction(
		"Closing Violin",
		"Musical Instruments",
		"A violin that will close very soon after creation",
		auction_entity.Used,
	)
	require.Nil(t, err)
	require.Nil(t, repo.CreateAuction(ctx, closing))

	// wait for the goroutine to close "closing"
	time.Sleep(2 * time.Second)

	completed, internalErr := repo.FindAuctions(ctx, auction_entity.Completed, "", "")
	assert.Nil(t, internalErr)

	ids := make([]string, 0)
	for _, a := range completed {
		ids = append(ids, a.Id)
	}
	assert.Contains(t, ids, closing.Id, "closed auction must appear in Completed filter")
}

func TestFindAuctions_FilterByCategory(t *testing.T) {
	ctx := context.Background()
	container, uri := startMongoContainer(ctx, t)
	defer container.Terminate(ctx)

	repo := newTestRepo(ctx, t, uri)

	guitar, err := auction_entity.CreateAuction(
		"Gibson Les Paul",
		"Guitars",
		"A classic Gibson Les Paul in sunburst finish",
		auction_entity.Used,
	)
	require.Nil(t, err)
	require.Nil(t, repo.CreateAuction(ctx, guitar))

	piano, err := auction_entity.CreateAuction(
		"Steinway Grand",
		"Pianos",
		"A Steinway concert grand piano in perfect condition",
		auction_entity.New,
	)
	require.Nil(t, err)
	require.Nil(t, repo.CreateAuction(ctx, piano))

	results, internalErr := repo.FindAuctions(ctx, 0, "Guitars", "")
	assert.Nil(t, internalErr)
	assert.Len(t, results, 1)
	assert.Equal(t, guitar.Id, results[0].Id)
	assert.Equal(t, "Guitars", results[0].Category)
}

// newTestRepo is a helper to build AuctionRepository connected to the test MongoDB.
func newTestRepo(ctx context.Context, t *testing.T, uri string) *AuctionRepository {
	t.Helper()
	client := newTestMongoClient(ctx, t, uri)
	return NewAuctionRepository(client.Database("auctions_test"))
}
