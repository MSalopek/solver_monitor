package monitor

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

type BlockTime struct {
	Height    int64  `json:"height"`
	Timestamp int64  `json:"timestamp"`
	Datetime  string `json:"datetime"`
}

const BLOCK_QUERY string = "/cosmos/base/tendermint/v1beta1/blocks"

var urls = []string{
	"https://osmosis-api.polkachu.com",          // doesn't have old block heights
	"https://rest.lavenderfive.com:443/osmosis", // doesn't have old block heights
	"https://osmosis-lcd.quickapi.com",
	"https://osmosis-rest.publicnode.com",
	"https://rest.cros-nest.com/osmosis",
	"https://osmosis-api.chainroot.io",
	"https://rest-osmosis.ecostake.com",
	"https://api.osmosis.validatus.com:443",
}

var RateLimitErr = errors.New("rate limit error")
var NotAvailableError = errors.New("server not available error")

func (m *Monitor) FetchAndSaveBlocktimes(intervalSeconds int) error {
	m.logger.Info().Msg("fetching blocktimes")
	// fetch heights that we don't already have stored
	rows, err := m.db.Query(`
	SELECT DISTINCT t.height 
	FROM tx_data t
	LEFT JOIN osmo_block_times b ON t.height = b.height
	WHERE b.height IS NULL`)
	if err != nil {
		return err
	}
	defer rows.Close()
	heights := []int64{}
	for rows.Next() {
		var h int64
		err := rows.Scan(&h)
		if err != nil {
			return fmt.Errorf("scan error: %w", err)
		}
		heights = append(heights, h)
	}
	m.logger.Info().Int64("count", int64(len(heights))).Msg("blocks in database")

	// process fetched block times
	var blocktimes = make(chan *BlockTime, 10)
	go func() {
		for b := range blocktimes {
			bt := b
			m.storeBlockTime(bt)
		}
	}()

	heightsChan := make(chan int64, len(heights))
	for _, h := range heights {
		heightsChan <- h
	}
	close(heightsChan)

	var wg sync.WaitGroup

	for _, url := range urls {
		wg.Add(1)
		go func(wg *sync.WaitGroup, apiUrl string) {
			defer wg.Done()
			for h := range heightsChan {
				time.Sleep(time.Duration(intervalSeconds) * time.Second)
				b, err := m.getBlockTimestamp(apiUrl, h)

				if err != nil && errors.Is(err, RateLimitErr) {
					m.logger.Warn().Int64("height", h).Str("URL", url).Msg("request was rate limited - sleeping for a minute")
					time.Sleep(1 * time.Minute)
					continue
				}

				// stop using this endpoint
				if err != nil && errors.Is(err, NotAvailableError) {
					m.logger.Error().Int64("height", h).Str("URL", url).Msg("server not available - stopping routine")
					break
				}

				if err != nil {
					m.logger.Error().Int64("height", h).Str("URL", url).Err(err).Msg("error getting block timestamp - skipping")
					break
				}

				m.logger.Debug().Int64("height", h).Str("URL", url).Msg("fetched block time")
				blocktimes <- b
			}
		}(&wg, url)
	}
	wg.Wait()
	m.logger.Info().Int("success", len(blocktimes)).Int("failed", len(heights)-len(blocktimes)).Msg("inserted block heights")

	return nil
}

func (m *Monitor) storeBlockTime(b *BlockTime) error {
	_, err := m.db.Exec(`
	INSERT INTO osmo_block_times (height, timestamp, datetime)
	VALUES (?, ?, ?)
`, b.Height, b.Timestamp, b.Datetime)
	if err != nil {
		return fmt.Errorf("failed to insert block time: %w", err)
	}

	m.logger.Debug().Int64("height", b.Height).Msg("inserted block time")
	return nil
}

// don't want to mess widh cosmos/comet encoders again...
type ShortBlockResp struct {
	Block struct {
		Header struct {
			Version struct {
				Block string `json:"block"`
				App   string `json:"app"`
			} `json:"version"`
			ChainID string    `json:"chain_id"`
			Height  string    `json:"height"`
			Time    time.Time `json:"time"`
		} `json:"header"`
	} `json:"block"`
}

func (m *Monitor) getBlockTimestamp(apiUrl string, height int64) (*BlockTime, error) {
	url := fmt.Sprintf("%s/%s/%d", apiUrl, BLOCK_QUERY, height)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 500 {
		return nil, NotAvailableError
	}

	if resp.StatusCode == 429 {
		return nil, RateLimitErr
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("got error code %d - url: %s", resp.StatusCode, url)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var data ShortBlockResp
	if err := json.Unmarshal(body, &data); err != nil {
		m.logger.Debug().Str("body", string(body)).Msg("error unmarshalling block response")
		return nil, err
	}

	m.logger.Debug().Int("block_height", int(height)).Msg("fetched osmosis block")
	b := &BlockTime{
		Height:    height,
		Datetime:  data.Block.Header.Time.UTC().Format("2006-01-02 15:04:05"),
		Timestamp: data.Block.Header.Time.Unix(),
	}
	return b, nil

}
