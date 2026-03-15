package auction_entity

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAuctionValidation(t *testing.T) {
	validName := "Fender Stratocaster"
	validCategory := "Musical Instruments"
	validDescription := "Original 1965 Fender Stratocaster in excellent condition"

	t.Run("valid auction passes", func(t *testing.T) {
		_, err := CreateAuction(validName, validCategory, validDescription, New)
		assert.Nil(t, err)
	})

	t.Run("short product name rejected", func(t *testing.T) {
		_, err := CreateAuction("X", validCategory, validDescription, New)
		assert.NotNil(t, err)
	})

	t.Run("short category rejected", func(t *testing.T) {
		_, err := CreateAuction(validName, "AB", validDescription, New)
		assert.NotNil(t, err)
	})

	t.Run("short description rejected", func(t *testing.T) {
		_, err := CreateAuction(validName, validCategory, "Too short", New)
		assert.NotNil(t, err)
	})

	t.Run("invalid condition rejected even with long description", func(t *testing.T) {
		_, err := CreateAuction(validName, validCategory, validDescription, ProductCondition(99))
		assert.NotNil(t, err, "condition=99 must be rejected regardless of description length")
	})

	t.Run("all valid conditions accepted", func(t *testing.T) {
		for _, cond := range []ProductCondition{New, Used, Refurbished} {
			_, err := CreateAuction(validName, validCategory, validDescription, cond)
			assert.Nil(t, err, "condition %d must be valid", cond)
		}
	})
}
