//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const baseURL = "http://localhost:8080"

// --- HTTP helpers ---

func post(t *testing.T, path string, body any) *http.Response {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	resp, err := http.Post(baseURL+path, "application/json", bytes.NewReader(raw))
	require.NoError(t, err)
	return resp
}

func get(t *testing.T, path string) *http.Response {
	t.Helper()
	resp, err := http.Get(baseURL + path)
	require.NoError(t, err)
	return resp
}

func decodeJSON(t *testing.T, resp *http.Response, out any) {
	t.Helper()
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	require.NoError(t, json.Unmarshal(body, out), "body: %s", body)
}

// --- DTOs ---

type createAuctionReq struct {
	ProductName string `json:"product_name"`
	Category    string `json:"category"`
	Description string `json:"description"`
	Condition   int    `json:"condition"`
}

type auctionResp struct {
	Id          string  `json:"id"`
	ProductName string  `json:"product_name"`
	Category    string  `json:"category"`
	Status      int     `json:"status"` // 0=Active 1=Completed
	Timestamp   string  `json:"timestamp"`
}

type bidReq struct {
	UserId    string  `json:"user_id"`
	AuctionId string  `json:"auction_id"`
	Amount    float64 `json:"amount"`
}

type bidResp struct {
	Id        string  `json:"id"`
	UserId    string  `json:"user_id"`
	AuctionId string  `json:"auction_id"`
	Amount    float64 `json:"amount"`
}

type winnerResp struct {
	Auction auctionResp `json:"auction"`
	Bid     bidResp     `json:"bid"`
}

type errResp struct {
	Message string `json:"message"`
	Err     string `json:"err"`
	Code    int    `json:"code"`
}

// --- Test suite ---

// TestE2E_FullAuctionLifecycle is the main end-to-end scenario:
// create → bid → auto-close → winner → reject late bid.
func TestE2E_FullAuctionLifecycle(t *testing.T) {
	userId := uuid.New().String()

	// ── 1. Create auction ────────────────────────────────────────────────────
	t.Run("1_CreateAuction", func(t *testing.T) {
		resp := post(t, "/auction", createAuctionReq{
			ProductName: "E2E Vintage Guitar",
			Category:    "Musical Instruments",
			Description: "A rare vintage guitar in excellent condition for E2E test",
			Condition:   2,
		})
		defer resp.Body.Close()
		assert.Equal(t, http.StatusCreated, resp.StatusCode)
	})

	// ── 2. Find active auction by listing ────────────────────────────────────
	var auction auctionResp
	t.Run("2_FindActiveAuction", func(t *testing.T) {
		resp := get(t, "/auction?status=0&productName=E2E+Vintage+Guitar")
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var auctions []auctionResp
		decodeJSON(t, resp, &auctions)

		var found bool
		for _, a := range auctions {
			if a.ProductName == "E2E Vintage Guitar" {
				auction = a
				found = true
				break
			}
		}
		require.True(t, found, "created auction must appear in active list")
		assert.Equal(t, 0, auction.Status, "new auction must be Active (status=0)")
		assert.NotEmpty(t, auction.Id)
	})

	// ── 3. Find auction by ID ─────────────────────────────────────────────────
	t.Run("3_FindAuctionById", func(t *testing.T) {
		resp := get(t, "/auction/"+auction.Id)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var a auctionResp
		decodeJSON(t, resp, &a)
		assert.Equal(t, auction.Id, a.Id)
		assert.Equal(t, "E2E Vintage Guitar", a.ProductName)
		assert.Equal(t, 0, a.Status)
	})

	// ── 4. Place multiple bids ────────────────────────────────────────────────
	bids := []float64{500.0, 1200.0, 750.0, 1800.0, 999.0}
	t.Run("4_PlaceBids", func(t *testing.T) {
		for _, amount := range bids {
			resp := post(t, "/bid", bidReq{
				UserId:    userId,
				AuctionId: auction.Id,
				Amount:    amount,
			})
			defer resp.Body.Close()
			assert.Equal(t, http.StatusCreated, resp.StatusCode,
				"bid of %.2f should be accepted", amount)
		}
	})

	// ── 5. Confirm bids were persisted ────────────────────────────────────────
	t.Run("5_ConfirmBidsPersisted", func(t *testing.T) {
		// BATCH_INSERT_INTERVAL=3s in .env; give the batch goroutine time to flush
		time.Sleep(5 * time.Second)

		resp := get(t, "/bid/"+auction.Id)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var storedBids []bidResp
		decodeJSON(t, resp, &storedBids)
		assert.Len(t, storedBids, len(bids),
			"all %d bids must be persisted after batch flush", len(bids))

		for _, b := range storedBids {
			assert.Equal(t, auction.Id, b.AuctionId)
			assert.Equal(t, userId, b.UserId)
			assert.Positive(t, b.Amount)
		}
	})

	// ── 6. Reject invalid bid payloads ────────────────────────────────────────
	t.Run("6_RejectInvalidBids", func(t *testing.T) {
		// 6a: invalid UUID for user_id
		resp := post(t, "/bid", map[string]any{
			"user_id":    "not-a-uuid",
			"auction_id": auction.Id,
			"amount":     100,
		})
		defer resp.Body.Close()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
			"non-UUID user_id must be rejected with 400")

		// 6b: negative amount
		resp2 := post(t, "/bid", map[string]any{
			"user_id":    uuid.New().String(),
			"auction_id": auction.Id,
			"amount":     -50,
		})
		defer resp2.Body.Close()
		assert.Equal(t, http.StatusBadRequest, resp2.StatusCode,
			"negative amount must be rejected with 400")

		// 6c: zero amount
		resp3 := post(t, "/bid", map[string]any{
			"user_id":    uuid.New().String(),
			"auction_id": auction.Id,
			"amount":     0,
		})
		defer resp3.Body.Close()
		assert.Equal(t, http.StatusBadRequest, resp3.StatusCode,
			"zero amount must be rejected with 400")
	})

	// ── 7. Wait for auto-close ────────────────────────────────────────────────
	// AUCTION_INTERVAL=20s; we already spent ~5s in step 5 so wait 18 more.
	t.Run("7_WaitForAutoClose", func(t *testing.T) {
		t.Log("waiting for AUCTION_INTERVAL to expire...")
		time.Sleep(18 * time.Second)

		resp := get(t, "/auction/"+auction.Id)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var a auctionResp
		decodeJSON(t, resp, &a)
		assert.Equal(t, 1, a.Status,
			"auction must be Completed (status=1) after AUCTION_INTERVAL expires")
	})

	// ── 8. Verify auction appears in completed filter ─────────────────────────
	t.Run("8_AuctionInCompletedList", func(t *testing.T) {
		resp := get(t, "/auction?status=1")
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var auctions []auctionResp
		decodeJSON(t, resp, &auctions)

		var found bool
		for _, a := range auctions {
			if a.Id == auction.Id {
				found = true
				assert.Equal(t, 1, a.Status)
			}
		}
		assert.True(t, found, "closed auction must appear in Completed (status=1) list")
	})

	// ── 9. Bid on closed auction returns 422 ──────────────────────────────────
	t.Run("9_BidOnClosedAuction_Returns422", func(t *testing.T) {
		resp := post(t, "/bid", bidReq{
			UserId:    userId,
			AuctionId: auction.Id,
			Amount:    99999.0,
		})
		defer resp.Body.Close()
		assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode,
			"bid on closed auction must return HTTP 422")

		var errBody errResp
		body, _ := io.ReadAll(resp.Body)
		_ = json.Unmarshal(body, &errBody)
		assert.Equal(t, "conflict", errBody.Err)
		assert.Contains(t, errBody.Message, "closed")

		// Confirm the late bid was NOT persisted
		resp2 := get(t, "/bid/"+auction.Id)
		var storedBids []bidResp
		decodeJSON(t, resp2, &storedBids)
		assert.Len(t, storedBids, len(bids),
			"bid count must remain %d after rejected late bid", len(bids))
	})

	// ── 10. Winning bid is highest amount ─────────────────────────────────────
	t.Run("10_WinningBid", func(t *testing.T) {
		resp := get(t, "/auction/winner/"+auction.Id)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var w winnerResp
		decodeJSON(t, resp, &w)
		assert.Equal(t, auction.Id, w.Auction.Id)
		assert.Equal(t, 1, w.Auction.Status, "winner auction must be Completed")
		assert.Equal(t, 1800.0, w.Bid.Amount, "winner must be the highest bid (1800)")
		assert.Equal(t, userId, w.Bid.UserId)
	})
}

// TestE2E_FilterAuctions validates listing with category and productName filters.
func TestE2E_FilterAuctions(t *testing.T) {
	// Create two auctions in different categories
	resp1 := post(t, "/auction", createAuctionReq{
		ProductName: "E2E Filter Telescope",
		Category:    "E2E_Astronomy",
		Description: "A high-quality refracting telescope for E2E filter test",
		Condition:   1,
	})
	resp1.Body.Close()
	require.Equal(t, http.StatusCreated, resp1.StatusCode)

	resp2 := post(t, "/auction", createAuctionReq{
		ProductName: "E2E Filter Microscope",
		Category:    "E2E_Biology",
		Description: "A precision optical microscope for E2E filter test",
		Condition:   1,
	})
	resp2.Body.Close()
	require.Equal(t, http.StatusCreated, resp2.StatusCode)

	time.Sleep(500 * time.Millisecond)

	// Filter by category
	t.Run("FilterByCategory", func(t *testing.T) {
		resp := get(t, "/auction?status=0&category=E2E_Astronomy")
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var auctions []auctionResp
		decodeJSON(t, resp, &auctions)
		require.NotEmpty(t, auctions)
		for _, a := range auctions {
			assert.Equal(t, "E2E_Astronomy", a.Category)
		}
	})

	// Filter by productName
	t.Run("FilterByProductName", func(t *testing.T) {
		resp := get(t, "/auction?status=0&productName=E2E+Filter+Microscope")
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var auctions []auctionResp
		decodeJSON(t, resp, &auctions)
		require.NotEmpty(t, auctions)
		assert.Equal(t, "E2E Filter Microscope", auctions[0].ProductName)
	})
}

// TestE2E_NotFoundAuction validates 404 for unknown IDs.
func TestE2E_NotFoundAuction(t *testing.T) {
	fakeId := uuid.New().String()

	t.Run("GetAuction_NotFound", func(t *testing.T) {
		resp := get(t, "/auction/"+fakeId)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("GetWinner_NotFound", func(t *testing.T) {
		resp := get(t, "/auction/winner/"+fakeId)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("BidOnNonExistentAuction_NotFound", func(t *testing.T) {
		resp := post(t, "/bid", bidReq{
			UserId:    uuid.New().String(),
			AuctionId: fakeId,
			Amount:    100,
		})
		defer resp.Body.Close()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode,
			"bid on non-existent auction must return 404")

		var errBody errResp
		body, _ := io.ReadAll(resp.Body)
		_ = json.Unmarshal(body, &errBody)
		assert.Equal(t, "not_found", errBody.Err)
	})
}

// TestE2E_InvalidAuctionPayload validates bad request handling on auction creation.
func TestE2E_InvalidAuctionPayload(t *testing.T) {
	cases := []struct {
		name    string
		payload createAuctionReq
	}{
		{
			name: "short product name",
			payload: createAuctionReq{
				ProductName: "X",
				Category:    "Electronics",
				Description: "A valid description long enough to pass",
				Condition:   1,
			},
		},
		{
			name: "short category",
			payload: createAuctionReq{
				ProductName: "Valid Product Name",
				Category:    "AB",
				Description: "A valid description long enough to pass",
				Condition:   1,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := post(t, "/auction", tc.payload)
			defer resp.Body.Close()
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
				"invalid auction payload must return 400")
		})
	}
}

// TestE2E_ShortAutoClose creates an auction against a fast-closing scenario by
// directly hitting the API and polling for status change.
// NOTE: This test assumes AUCTION_INTERVAL=20s (from docker-compose .env).
func TestE2E_ShortAutoClose(t *testing.T) {
	resp := post(t, "/auction", createAuctionReq{
		ProductName: "E2E Short AutoClose Item",
		Category:    "AutoClose Tests",
		Description: "Testing the goroutine-based auto close mechanism end to end",
		Condition:   1,
	})
	resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	// Find the auction
	time.Sleep(500 * time.Millisecond)
	listResp := get(t, "/auction?status=0&productName=E2E+Short+AutoClose+Item")
	var auctions []auctionResp
	decodeJSON(t, listResp, &auctions)

	var auction auctionResp
	for _, a := range auctions {
		if a.ProductName == "E2E Short AutoClose Item" {
			auction = a
		}
	}
	require.NotEmpty(t, auction.Id, "must find the created auction")

	// Immediately it must be Active
	assert.Equal(t, 0, auction.Status, "auction must start as Active")

	// Poll until Completed (max 30s to account for AUCTION_INTERVAL=20s)
	deadline := time.Now().Add(30 * time.Second)
	var finalStatus int
	for time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)
		r := get(t, "/auction/"+auction.Id)
		var a auctionResp
		decodeJSON(t, r, &a)
		finalStatus = a.Status
		if finalStatus == 1 {
			break
		}
	}

	assert.Equal(t, 1, finalStatus,
		fmt.Sprintf("auction %s must auto-close within 30s", auction.Id))
}
