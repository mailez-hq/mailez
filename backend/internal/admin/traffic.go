// Traffic reporting: the backend samples the engine's Prometheus counters
// every few minutes into traffic_points, and /admin/traffic aggregates the
// per-day deltas (engine restarts reset counters; a decreasing value counts
// as an absolute restart baseline instead of negative traffic).
package admin

import (
	"bufio"
	"context"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"mailez/backend/internal/cluster"
	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
)

var metricLine = regexp.MustCompile(`^([a-zA-Z_:][a-zA-Z0-9_:]*)(\{[^}]*\})?\s+([0-9.eE+-]+)$`)
var labelRe = regexp.MustCompile(`([a-zA-Z_]+)="([^"]*)"`)

// TrafficSampler scrapes the engine /metrics endpoint periodically.
type TrafficSampler struct {
	DB     *gorm.DB
	Cfg    core.Config
	Client *http.Client
}

// NewTrafficSampler wires the sampler.
func NewTrafficSampler(db *gorm.DB, cfg core.Config) *TrafficSampler {
	return &TrafficSampler{
		DB:  db,
		Cfg: cfg,
		Client: &http.Client{
			Timeout: 8 * time.Second,
		},
	}
}

// Run samples until cancelled.
func (s *TrafficSampler) Run(ctx context.Context) {
	s.sample()
	t := time.NewTicker(5 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.sample()
		}
	}
}

func (s *TrafficSampler) sample() {
	if !cluster.TryHold(s.DB, "admin_traffic", cluster.LeaseTTL) {
		return
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, s.Cfg.EngineMetricsURL, nil)
	if err != nil {
		return
	}
	resp, err := s.Client.Do(req)
	if err != nil {
		log.Printf("traffic sampler: scrape: %v", err)
		return
	}
	defer resp.Body.Close()
	point, err := parsePrometheus(resp.Body)
	if err != nil {
		log.Printf("traffic sampler: parse: %v", err)
		return
	}
	point.TS = time.Now().UTC()
	if err := s.DB.Create(point).Error; err != nil {
		log.Printf("traffic sampler: insert: %v", err)
	}
}

// parsePrometheus extracts the mailezine counters the report needs.
func parsePrometheus(r io.Reader) (*models.TrafficPoint, error) {
	point := &models.TrafficPoint{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		m := metricLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		value, err := strconv.ParseFloat(m[3], 64)
		if err != nil {
			continue
		}
		labels := map[string]string{}
		for _, kv := range labelRe.FindAllStringSubmatch(m[2], -1) {
			labels[kv[1]] = kv[2]
		}
		switch m[1] {
		case "mailezine_smtp_messages_in_total":
			switch labels["outcome"] {
			case "accepted":
				point.InAccepted = int64(value)
			case "rejected":
				point.InRejected = int64(value)
			case "deferred":
				point.InDeferred = int64(value)
			}
		case "mailezine_queue_messages_total":
			switch labels["event"] {
			case "delivered":
				point.OutDelivered = int64(value)
			case "bounced":
				point.OutBounced = int64(value)
			case "deferred":
				point.OutDeferred = int64(value)
			}
		case "mailezine_queue_depth":
			point.QueueDepth += int64(value)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return point, nil
}

// registerTraffic mounts the report endpoint.
func (h *Handler) registerTraffic(r fiber.Router, mw fiber.Handler) {
	r.Get("/admin/traffic", mw, h.trafficReport)
}

// TrafficDay is one aggregated day of traffic.
type TrafficDay struct {
	Date         string `json:"date"`
	InAccepted   int64  `json:"in_accepted"`
	InRejected   int64  `json:"in_rejected"`
	InDeferred   int64  `json:"in_deferred"`
	OutDelivered int64  `json:"out_delivered"`
	OutBounced   int64  `json:"out_bounced"`
	OutDeferred  int64  `json:"out_deferred"`
}

// trafficReport returns per-day traffic deltas for the last N days.
// @Summary Traffic report
// @Tags admin
// @Produce json
// @Param days query int false "days back (default 14, max 90)"
// @Success 200 {object} map[string]interface{}
// @Router /admin/traffic [get]
func (h *Handler) trafficReport(c *fiber.Ctx) error {
	days := c.QueryInt("days", 14)
	if days < 1 {
		days = 1
	}
	if days > 90 {
		days = 90
	}
	since := time.Now().UTC().AddDate(0, 0, -days)
	var points []models.TrafficPoint
	if err := h.DB.Where("ts >= ?", since).Order("ts").Find(&points).Error; err != nil {
		return fiberError(c, err, "load traffic failed")
	}
	if len(points) == 0 {
		return c.JSON(fiber.Map{"days": []TrafficDay{}, "latest_queue_depth": 0, "sampling": h.Cfg.EngineMetricsURL != ""})
	}

	// Cumulative counters -> per-day deltas with restart detection: a value
	// that drops below its predecessor counts as the new absolute baseline.
	byDay := map[string]*TrafficDay{}
	prev := points[0]
	var latestQueue int64
	for _, p := range points[1:] {
		date := p.TS.Local().Format("2006-01-02")
		day := byDay[date]
		if day == nil {
			day = &TrafficDay{Date: date}
			byDay[date] = day
		}
		day.InAccepted += delta(p.InAccepted, prev.InAccepted)
		day.InRejected += delta(p.InRejected, prev.InRejected)
		day.InDeferred += delta(p.InDeferred, prev.InDeferred)
		day.OutDelivered += delta(p.OutDelivered, prev.OutDelivered)
		day.OutBounced += delta(p.OutBounced, prev.OutBounced)
		day.OutDeferred += delta(p.OutDeferred, prev.OutDeferred)
		latestQueue = p.QueueDepth
		prev = p
	}
	out := make([]TrafficDay, 0, len(byDay))
	for _, day := range byDay {
		out = append(out, *day)
	}
	// Deterministic chronological order.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Date < out[j-1].Date; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return c.JSON(fiber.Map{
		"days":               out,
		"latest_queue_depth": latestQueue,
		"sampling":           h.Cfg.EngineMetricsURL != "",
	})
}

func delta(cur, prev int64) int64 {
	if cur < prev {
		// Counter reset (engine restart): treat as absolute.
		return cur
	}
	return cur - prev
}

func fiberError(c *fiber.Ctx, err error, msg string) error {
	return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": msg, "detail": err.Error()})
}
