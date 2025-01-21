package monitor

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type CoinGeckoAPIResponseETH struct {
	Ethereum struct {
		USD float64 `json:"usd"`
	} `json:"ethereum"`
}

type CoinGeckoAPIResponseOSMO struct {
	Osmosis struct {
		USD float64 `json:"usd"`
	} `json:"osmosis"`
}

func (m *Monitor) GetCoingeckoPrices() (error) {
	m.logger.Info().Msg("Fetching ETH USD prices from CoinGecko")
	
	denoms := []string{"ethereum", "osmosis"}
	for _, denom := range denoms {
		EthPrice, err := fetchPrice(denom)
		if err != nil {
			m.logger.Error().Err(err).Msgf("Failed to fetch ETH price for %s", denom)
			return err
		}

		err = m.InsertUsdPrice(denom, EthPrice)
		if err != nil {
			m.logger.Error().Err(err).Msg("Failed to store ETH price in database")
			return err
		}
		m.logger.Info().Msgf("Fetched and stored ETH price for %s", denom)
	}
	return nil
}

func fetchPrice(denom string) (float64, error) {

	url := fmt.Sprintf("https://api.coingecko.com/api/v3/simple/price?ids=%s&vs_currencies=usd", denom)

	// Make HTTP GET request
	resp, err := http.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	switch denom {
	case "ethereum":
		var apiResponse CoinGeckoAPIResponseETH
		err = json.Unmarshal(body, &apiResponse)
		if err != nil {
			return 0, err
		}
		return apiResponse.Ethereum.USD, nil
	case "osmosis":
		var apiResponse CoinGeckoAPIResponseOSMO
		err = json.Unmarshal(body, &apiResponse)
		if err != nil {
			return 0, err
		}
		return apiResponse.Osmosis.USD, nil
	default:
		return 0, fmt.Errorf("unsupported denom: %s", denom)
	}
}