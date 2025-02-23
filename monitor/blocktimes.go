package monitor

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

type BlockTime struct {
	Height    int64  `json:"height"`
	Timestamp int64  `json:"timestamp"`
	Datetime  string `json:"datetime"`
}

const BLOCK_QUERY string = "/cosmos/base/tendermint/v1beta1/blocks"

var RateLimitErr = errors.New("rate limit error")

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

	var blocktimes = make([]*BlockTime, 0, len(heights))
	for _, h := range heights {
		time.Sleep(time.Duration(intervalSeconds) * time.Second)

		b, err := m.getBlockTimestamp(h)

		if err != nil && errors.Is(err, RateLimitErr) {
			m.logger.Warn().Int64("height", h).Msg("request was rate limited - sleeping for a minute")
			time.Sleep(1 * time.Minute)
			continue
		}

		if err != nil {
			m.logger.Error().Int64("height", h).Err(err).Msg("error getting block timestamp - skipping")
			continue
		}

		m.logger.Debug().Int64("height", h).Msg("fetched block time")
		_, err = m.db.Exec(`
		INSERT INTO osmo_block_times (height, timestamp, datetime)
		VALUES (?, ?, ?)
	`, b.Height, b.Timestamp, b.Datetime)
		if err != nil {
			return fmt.Errorf("failed to insert block time: %w", err)
		}

		m.logger.Debug().Int64("height", h).Msg("inserted block time")
		blocktimes = append(blocktimes, b)
	}

	m.logger.Info().Int("success", len(blocktimes)).Int("failed", len(heights)-len(blocktimes)).Msg("inserted block heights")

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

func (m *Monitor) getBlockTimestamp(height int64) (*BlockTime, error) {
	url := fmt.Sprintf("%s/%s/%d", m.cfg.Osmosis.ApiUrl, BLOCK_QUERY, height)
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

	if resp.StatusCode == 429 {
		return nil, RateLimitErr
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("got error code %d", resp.StatusCode)
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
