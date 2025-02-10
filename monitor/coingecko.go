package monitor

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	COINGECKO_AVALANCHE_ID = "avalanche-2"
	COINGECKO_OSMOSIS_ID   = "osmosis"
	COINGECKO_ETHEREUM_ID  = "ethereum"
)

type UsdPrice struct {
	USD float64 `json:"usd"`
}

// Response example from API:
//
//	{
//	    "ethereum": {
//	        "usd": 3265.89
//	    },
//	    "osmosis": {
//	        "usd": 0.40659
//	    }
//	}
type PriceResponse map[string]UsdPrice

func (m *Monitor) GetCoingeckoPrices() error {
	m.logger.Info().Msg("Fetching USD prices from CoinGecko")

	denoms := []string{
		COINGECKO_ETHEREUM_ID,
		COINGECKO_OSMOSIS_ID,
		COINGECKO_AVALANCHE_ID,
	}
	denomString := strings.Join(denoms, ",")
	prices, err := fetchPrice(denomString)
	if err != nil {
		m.logger.Error().Err(err).Msgf("Failed to fetch prices for %s", denomString)
		return err
	}

	for denom, price := range prices {
		err = m.InsertUsdPrice(denom, price.USD)
		if err != nil {
			m.logger.Error().Err(err).Msg("Failed to store ETH price in database")
			return err
		}
		m.logger.Info().Msgf("Fetched and stored ETH price for %s", denom)
	}
	return nil
}

// Accepts a comma separated list of denoms (e.g. "ethereum,osmosis")
func fetchPrice(denoms string) (PriceResponse, error) {
	url := fmt.Sprintf("https://api.coingecko.com/api/v3/simple/price?ids=%s&vs_currencies=usd", denoms)

	// Make HTTP GET request
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var priceResponse PriceResponse
	err = json.Unmarshal(body, &priceResponse)
	if err != nil {
		return nil, err
	}
	return priceResponse, nil
}
