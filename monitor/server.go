package monitor

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

type Server struct {
	monitor *Monitor
}

func NewServer(monitor *Monitor) *Server {
	return &Server{
		monitor: monitor,
	}
}

func (s *Server) RunWithContext(ctx context.Context, addr string) error {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())

	// Setup routes
	router.GET("/balances/latest", s.getLatestBalances)
	router.GET("/balances/range", s.getBalancesInTimeRange)
	router.GET("/stats/orders_filled", s.getOrdersFilledStats)
	// TODO: needs pagination so I'm temporarily removing this
	// router.GET("/stats/fees", s.getFeesStats)

	srv := &http.Server{
		Addr:    addr,
		Handler: router,
	}

	// Graceful server shutdown
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			s.monitor.logger.Error().Err(err).Msg("server shutdown error")
		}
	}()

	// Start server
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}

	return nil
}

func (s *Server) getLatestBalances(c *gin.Context) {
	network := c.Query("network")
	asInteger := c.Query("as_integer")

	balances, err := s.monitor.GetDbLatestBalances(network)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get balances"})
		return
	}

	if asInteger == "" {
		for i := range balances {
			balanceDecimal, err := decimal.NewFromString(balances[i].Balance)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "unexpected error"})
				return
			}
			balances[i].Balance = balanceDecimal.Shift(-int32(balances[i].Exponent)).String()
		}
	}

	c.JSON(http.StatusOK, gin.H{"balances": balances})
}

func (s *Server) getBalancesInTimeRange(c *gin.Context) {
	from := c.Query("from")
	to := c.Query("to")
	network := c.Query("network")

	if network == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "network is required"})
		return
	}

	var fromTime, toTime time.Time
	var err error

	if from != "" {
		fromTime, err = time.Parse("2006-01-02", from)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid from date format, expected YYYY-MM-DD"})
			return
		}
	}

	if to != "" {
		toTime, err = time.Parse("2006-01-02", to)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid to date format, expected YYYY-MM-DD"})
			return
		}
	}

	balances, err := s.monitor.GetDbBalancesInTimeRange(network, fromTime, toTime)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get balances"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"balances": balances})
}

type OrdersFilledStatsResponse struct {
	TotalSolverRevenue string                 `json:"total_solver_revenue"`
	TotalOrderCount    string                 `json:"total_order_count"`
	NetworkStats       []NetworkStatsResponse `json:"networks"`
}

type NetworkStatsResponse struct {
	TotalSolverRevenue string `json:"total_solver_revenue"`
	OrderCount         string `json:"order_count"`
	Network            string `json:"network"`
}

func toOrdersFilledStatsResponse(stats *OrderStatsSummary) OrdersFilledStatsResponse {
	return OrdersFilledStatsResponse{
		TotalSolverRevenue: strconv.FormatInt(stats.TotalSolverRevenue, 10),
		TotalOrderCount:    strconv.FormatInt(stats.TotalOrderCount, 10),
		NetworkStats:       toNetworkStatsResponse(stats.NetworkOrderStats),
	}
}

func toNetworkStatsResponse(stats []NetworkOrderStats) []NetworkStatsResponse {
	networkStats := []NetworkStatsResponse{}
	for _, stat := range stats {
		chainName, ok := ChainIdToNetwork[stat.Network]
		if !ok {
			chainName = stat.Network
		}
		networkStats = append(networkStats, NetworkStatsResponse{
			TotalSolverRevenue: strconv.FormatInt(stat.TotalSolverRevenue, 10),
			OrderCount:         strconv.FormatInt(stat.OrderCount, 10),
			Network:            chainName,
		})
	}
	return networkStats
}

// if from and to are not provided, the stats are aggregated over all records
func (s *Server) getOrdersFilledStats(c *gin.Context) {
	asInteger := c.Query("as_integer")
	filler := c.Query("filler")
	if filler == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "filler address is required"})
		return
	}

	stats, err := s.monitor.GetDbFilledOrderStats(filler)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get stats"})
		return
	}

	resp := toOrdersFilledStatsResponse(stats)
	if asInteger == "" {
		resp.TotalSolverRevenue = decimal.NewFromInt(stats.TotalSolverRevenue).Shift(-6).String()
		for i := range resp.NetworkStats {
			totalSolverRevenue, err := decimal.NewFromString(resp.NetworkStats[i].TotalSolverRevenue)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "unexpected error"})
				return
			}
			resp.NetworkStats[i].TotalSolverRevenue = totalSolverRevenue.Shift(-6).String()
		}
	}

	c.JSON(http.StatusOK, gin.H{"orders_filled": resp})
}

func (s *Server) getFeesStats(c *gin.Context) {
	asInteger := c.Query("as_integer")

	stats, err := s.monitor.GetDbFeesStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get stats"})
		return
	}

	if asInteger == "" {
		totalDecimal, err := decimal.NewFromString(stats.TotalGasUsed)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "unexpected error"})
			s.monitor.logger.Error().Err(err).Msg("failed to convert total gas used to decimal")
			return
		}
		// this is eth
		stats.TotalGasUsed = totalDecimal.Shift(-18).String()
		for i := range stats.NetworkStats {
			networkTotalDecimal, err := decimal.NewFromString(stats.NetworkStats[i].TotalGasUsed)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "unexpected error"})
				return
			}
			stats.NetworkStats[i].TotalGasUsed = networkTotalDecimal.Shift(-18).String()
		}
	}

	// Parameters are optional, pass them to monitor as is
	c.JSON(http.StatusOK, gin.H{"fees": stats})
}
