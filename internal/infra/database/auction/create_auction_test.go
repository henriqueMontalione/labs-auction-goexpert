package auction

import (
	"context"
	"fmt"
	"fullcycle-auction_go/internal/entity/auction_entity"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// startMongoContainer starts a MongoDB container and returns it with the connection URI.
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

// newTestMongoClient returns a connected *mongo.Client registered for cleanup.
func newTestMongoClient(ctx context.Context, t *testing.T, uri string) *mongo.Client {
	t.Helper()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatalf("failed to connect to mongo: %v", err)
	}
	t.Cleanup(func() { client.Disconnect(ctx) })
	return client
}

func TestCreateAuctionAutoClose(t *testing.T) {
	ctx := context.Background()

	container, uri := startMongoContainer(ctx, t)
	defer container.Terminate(ctx)

	t.Setenv("AUCTION_INTERVAL", "2s")

	repo := newTestRepo(ctx, t, uri)

	auctionEntity, internalErr := auction_entity.CreateAuction(
		"Vintage Guitar",
		"Music Instruments",
		"A rare vintage guitar in excellent condition",
		auction_entity.Used,
	)
	require.Nil(t, internalErr)

	internalErr = repo.CreateAuction(ctx, auctionEntity)
	require.Nil(t, internalErr)

	// Immediately after creation the auction must be Active
	found, internalErr := repo.FindAuctionById(ctx, auctionEntity.Id)
	assert.Nil(t, internalErr)
	assert.Equal(t, auction_entity.Active, found.Status, "auction should be Active right after creation")

	// Wait past the configured duration (2s + 1s buffer)
	time.Sleep(3 * time.Second)

	// The goroutine must have fired and updated the status to Completed
	found, internalErr = repo.FindAuctionById(ctx, auctionEntity.Id)
	assert.Nil(t, internalErr)
	assert.Equal(t, auction_entity.Completed, found.Status, "auction should be Completed after AUCTION_INTERVAL expires")
}
