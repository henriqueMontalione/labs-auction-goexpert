package auction

import (
	"context"
	"fmt"
	"fullcycle-auction_go/internal/entity/auction_entity"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func startMongoContainer(ctx context.Context, t *testing.T) (container testcontainers.Container, uri string) {
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

	uri = fmt.Sprintf("mongodb://%s:%s", host, port.Port())
	return container, uri
}

func TestCreateAuctionAutoClose(t *testing.T) {
	ctx := context.Background()

	container, uri := startMongoContainer(ctx, t)
	defer container.Terminate(ctx)

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatalf("failed to connect to mongo: %v", err)
	}
	defer client.Disconnect(ctx)

	// Use a short duration so the test runs fast
	t.Setenv("AUCTION_INTERVAL", "2s")

	database := client.Database("auctions_test")
	repo := NewAuctionRepository(database)

	auctionEntity, internalErr := auction_entity.CreateAuction(
		"Vintage Guitar",
		"Music Instruments",
		"A rare vintage guitar in excellent condition",
		auction_entity.Used,
	)
	assert.Nil(t, internalErr)
	assert.NotNil(t, auctionEntity)

	// Create the auction — this also launches the close goroutine
	internalErr = repo.CreateAuction(ctx, auctionEntity)
	assert.Nil(t, internalErr)

	// Immediately after creation the auction must be Active
	found, internalErr := repo.FindAuctionById(ctx, auctionEntity.Id)
	assert.Nil(t, internalErr)
	assert.Equal(t, auction_entity.Active, found.Status, "auction should be Active right after creation")

	// Wait past the configured duration (2s + 1s buffer)
	time.Sleep(3 * time.Second)

	// Now the goroutine should have fired and updated the status to Completed
	found, internalErr = repo.FindAuctionById(ctx, auctionEntity.Id)
	assert.Nil(t, internalErr)
	assert.Equal(t, auction_entity.Completed, found.Status, "auction should be Completed after AUCTION_INTERVAL expires")
}
