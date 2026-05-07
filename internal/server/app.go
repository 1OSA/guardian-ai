package server

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"guardian-ai/internal/cache"
	"guardian-ai/internal/config"

	pb "github.com/1OSA/guardian-ai/proto"

	"github.com/miekg/dns"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	_ "modernc.org/sqlite"
)

// ════════════════════════════════════════════════════════════════════════════════
// ─ Package-level constants and helpers ────────────────────────────────────────
// ════════════════════════════════════════════════════════════════════════════════

const (
	DefaultDBPath          = "guardian.db"
	DefaultBlockfile       = "blocklists/hosts.txt"
	DNSDefaultTTL          = 300
	MinTTL                 = 30
	MaxTTL                 = 86400
	DNSTimeout             = 5 * time.Second
	ServerStartupWait      = 500 * time.Millisecond
	HTTPBlockedByBlocklist = 418
	HTTPBlockedByML        = 419
	HTTPBlockedByRule      = 420

	logPrefixGuardian = "[guardian]"
	logPrefixDNS      = "[dns]"
	logPrefixHTTP     = "[http]"
	logPrefixDB       = "[db]"
	logPrefixML       = "[ml]"
	logPrefixCache    = "[cache]"
	logPrefixAuth     = "[auth]"
	logPrefixUPS      = "[upstreams]"
	logPrefixUI       = "[ui]"
)

var (
	LogLevelNames = cache.LogLevelNames[:]
	LogLevelError = cache.LogLevelError
	LogLevelWarn  = cache.LogLevelWarn
	LogLevelInfo  = cache.LogLevelInfo
	LogLevelDebug = cache.LogLevelDebug
)

type LogLevel = cache.LogLevel

// ════════════════════════════════════════════════════════════════════════════════
// ─ GuardianServer struct ──────────────────────────────────────────────────────
// ════════════════════════════════════════════════════════════════════════════════

type GuardianServer struct {
	db                   *sql.DB
	dbPath               string
	dbMu                 sync.RWMutex
	upstreams            []string
	upstreamsMu          sync.RWMutex
	blocklistFile        string
	blocklist            map[string]bool
	blocklistMu          sync.RWMutex
	dnsCache             *cache.LRUCache[string, *dns.Msg]
	dnsResponseTTL       map[string]uint32
	dnsCacheMu           sync.RWMutex
	mlCache              *cache.LRUCache[string, cache.MLCacheEntry]
	mlCacheMu            sync.RWMutex
	mlClient             pb.GuardianAIClient
	mlConn               *grpc.ClientConn
	rateLimiters         map[string]*cache.TokenBucket
	rateLimitMu          sync.RWMutex
	sessions             map[string]*UserSession
	sessionMu            sync.RWMutex
	svcScheduleCache     map[string]ServiceSchedule
	svcScheduleCacheMu   sync.RWMutex
	lastSvcScheduleFetch time.Time
	embeddedDist         embed.FS
	logLevel             LogLevel
	blocklistRegex       *regexp.Regexp
	stats                ServerStats
	statsMu              sync.RWMutex
	feedbackTotal        int64
	feedbackSafe         int64
	feedbackMal          int64
	feedbackMu           sync.RWMutex
	ctx                  context.Context
	cancel               context.CancelFunc
	appVersion           string
	mlThreshold          float64
	mlThresholdMu        sync.RWMutex
}

type UserSession struct {
	UserID    string
	Username  string
	IsAdmin   bool
	CreatedAt time.Time
	ExpiresAt time.Time
}

type ServerStats struct {
	TotalQueries     int64
	BlockedQueries   int64
	AllowedQueries   int64
	MLClassified     int64
	BlocklistMatches int64
	UpstreamErrors   int64
	CacheHits        int64
	CacheMisses      int64
	LastUpdateTime   time.Time
}

// ════════════════════════════════════════════════════════════════════════════════
// ─ NewGuardianServer constructor ──────────────────────────────────────────────
// ════════════════════════════════════════════════════════════════════════════════

func NewGuardianServer(
	upstream string,
	mlAddr string,
	blockfile string,
	dbPath string,
	logLevel LogLevel,
	embeddedDist embed.FS,
	appVersion string,
) (*GuardianServer, error) {
	ctx, cancel := context.WithCancel(context.Background())

	srv := &GuardianServer{
		dbPath:           dbPath,
		blocklistFile:    blockfile,
		blocklist:        make(map[string]bool),
		dnsCache:         cache.NewLRUCache[string, *dns.Msg](config.MaxDNSCacheSize),
		dnsResponseTTL:   make(map[string]uint32),
		mlCache:          cache.NewLRUCache[string, cache.MLCacheEntry](config.MaxMLCacheSize),
		rateLimiters:     make(map[string]*cache.TokenBucket),
		sessions:         make(map[string]*UserSession),
		svcScheduleCache: make(map[string]ServiceSchedule),
		embeddedDist:     embeddedDist,
		logLevel:         logLevel,
		feedbackTotal:    0,
		feedbackSafe:     0,
		feedbackMal:      0,
		ctx:              ctx,
		cancel:           cancel,
		appVersion:       appVersion,
		mlThreshold:      0.9,
	}

	upstreamAddrs := strings.Fields(upstream)
	if len(upstreamAddrs) == 0 {
		return nil, errors.New("no upstream DNS servers specified")
	}
	srv.upstreams = upstreamAddrs
	srv.Logf(LogLevelInfo, logPrefixUPS, "using upstreams: %v", upstreamAddrs)

	if err := srv.initDatabase(); err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	if err := srv.reloadAllSources(); err != nil {
		srv.Logf(LogLevelWarn, logPrefixGuardian, "failed to load blocklist (will retry): %v", err)
	}

	// Load ML threshold from settings table if it exists
	if thresholdStr, err := srv.getSetting("ml_threshold"); err == nil && thresholdStr != "" {
		if threshold, err := strconv.ParseFloat(thresholdStr, 64); err == nil {
			srv.mlThresholdMu.Lock()
			srv.mlThreshold = threshold
			srv.mlThresholdMu.Unlock()
			srv.Logf(LogLevelInfo, logPrefixML, "loaded ML threshold from settings: %.2f", threshold)
		}
	}

	if err := srv.connectMLService(mlAddr); err != nil {
		srv.Logf(LogLevelWarn, logPrefixML, "ML service unavailable (classification disabled): %v", err)
	}

	srv.Logf(LogLevelInfo, logPrefixGuardian, "GuardianServer initialized")
	return srv, nil
}

// ════════════════════════════════════════════════════════════════════════════════
// ─ Database initialization and schema ─────────────────────────────────────────
// ════════════════════════════════════════════════════════════════════════════════

func (srv *GuardianServer) initDatabase() error {
	db, err := sql.Open("sqlite", srv.dbPath)
	if err != nil {
		return err
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		return err
	}

	srv.db = db

	if err := srv.runMigrations(); err != nil {
		return err
	}

	srv.Logf(LogLevelInfo, logPrefixDB, "database initialized at %s", srv.dbPath)
	return nil
}

func (srv *GuardianServer) runMigrations() error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			username TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			is_admin BOOLEAN DEFAULT 0,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			last_login TIMESTAMP
		)`,

		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			username TEXT NOT NULL,
			is_admin BOOLEAN DEFAULT 0,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			expires_at TIMESTAMP NOT NULL,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,

		`CREATE TABLE IF NOT EXISTS queries (
			id TEXT PRIMARY KEY,
			domain TEXT NOT NULL,
			client_ip TEXT,
			query_type TEXT,
			result TEXT,
			blocked_by TEXT,
			is_malicious BOOLEAN,
			ml_category TEXT,
			ml_confidence REAL,
			response_time_ms REAL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			feedback_verdict TEXT,
			feedback_confidence REAL,
			feedback_timestamp TIMESTAMP
		)`,

		`CREATE TABLE IF NOT EXISTS blocklist_sources (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			url TEXT UNIQUE NOT NULL,
			last_updated TIMESTAMP,
			entry_count INTEGER DEFAULT 0,
			enabled BOOLEAN DEFAULT 1,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,

		`CREATE TABLE IF NOT EXISTS rules (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			condition_type TEXT,
			condition_value TEXT,
			action TEXT,
			priority INTEGER DEFAULT 0,
			enabled BOOLEAN DEFAULT 1,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,

		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,

		`CREATE TABLE IF NOT EXISTS service_schedules (
			id TEXT PRIMARY KEY,
			service_id TEXT NOT NULL,
			day_of_week INTEGER,
			start_hour INTEGER,
			end_hour INTEGER,
			enabled BOOLEAN DEFAULT 1,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,

		`CREATE TABLE IF NOT EXISTS client_groups (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,

		`CREATE TABLE IF NOT EXISTS client_group_members (
			id TEXT PRIMARY KEY,
			group_id TEXT NOT NULL,
			client_ip TEXT NOT NULL,
			added_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (group_id) REFERENCES client_groups(id) ON DELETE CASCADE
		)`,

		`CREATE INDEX IF NOT EXISTS idx_queries_domain ON queries(domain)`,
		`CREATE INDEX IF NOT EXISTS idx_queries_created_at ON queries(created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_queries_client_ip ON queries(client_ip)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at)`,
		`CREATE INDEX IF NOT EXISTS idx_blocklist_sources_enabled ON blocklist_sources(enabled)`,
		`CREATE INDEX IF NOT EXISTS idx_service_schedules_service_id ON service_schedules(service_id)`,
		`CREATE INDEX IF NOT EXISTS idx_client_group_members_group_id ON client_group_members(group_id)`,
	}

	// Run base migrations
	for _, migration := range migrations {
		if _, err := srv.db.ExecContext(srv.ctx, migration); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
	}

	// Run incremental alterations (ignore "duplicate column" errors)
	alterations := []string{
		`ALTER TABLE service_schedules ADD COLUMN scope TEXT DEFAULT 'global'`,
		`ALTER TABLE service_schedules ADD COLUMN scope_key TEXT DEFAULT ''`,
	}
	for _, alt := range alterations {
		_, _ = srv.db.ExecContext(srv.ctx, alt)
	}

	return nil
}

// ════════════════════════════════════════════════════════════════════════════════
// ─ ML service connection ──────────────────────────────────────────────────────
// ════════════════════════════════════════════════════════════════════════════════

func (srv *GuardianServer) connectMLService(mlAddr string) error {
	maxRetries := 10
	retryDelay := 500 * time.Millisecond

	srv.Logf(LogLevelInfo, logPrefixML, "attempting to connect to ML service at %s (max %d attempts)", mlAddr, maxRetries)

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			srv.Logf(LogLevelDebug, logPrefixML, "retrying ML service connection (attempt %d/%d) after %v...", attempt, maxRetries, retryDelay)
			time.Sleep(retryDelay)
		}

		conn, err := grpc.NewClient(
			mlAddr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithTimeout(3*time.Second),
		)
		if err != nil {
			srv.Logf(LogLevelDebug, logPrefixML, "connection attempt %d failed: %v", attempt+1, err)
			if attempt < maxRetries-1 {
				continue
			}
			return fmt.Errorf("failed to establish gRPC connection to ML service after %d attempts: %w", maxRetries, err)
		}

		srv.mlConn = conn
		srv.mlClient = pb.NewGuardianAIClient(conn)

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		req := &pb.DomainRequest{Domain: "google.com"}
		if _, err := srv.mlClient.PredictDomain(ctx, req); err != nil {
			if attempt < maxRetries-1 {
				conn.Close()
				continue
			}
			conn.Close()
			srv.mlConn = nil
			srv.mlClient = nil
			return fmt.Errorf("ML service health check failed after %d attempts: %w", maxRetries, err)
		}

		srv.Logf(LogLevelInfo, logPrefixML, "successfully connected to ML service at %s (attempt %d/%d)", mlAddr, attempt+1, maxRetries)
		return nil
	}

	return fmt.Errorf("failed to connect to ML service after %d attempts - service may not be running", maxRetries)
}

// ════════════════════════════════════════════════════════════════════════════════
// ─ DNS query handling ─────────────────────────────────────────────────────────
// ════════════════════════════════════════════════════════════════════════════════

func (srv *GuardianServer) handleDNSQuery(w dns.ResponseWriter, req *dns.Msg) {
	if len(req.Question) == 0 {
		srv.Logf(LogLevelDebug, logPrefixDNS, "received empty DNS query")
		return
	}

	remoteAddr := w.RemoteAddr().String()
	clientIP := strings.Split(remoteAddr, ":")[0]

	if !srv.checkRateLimit(clientIP) {
		srv.Logf(LogLevelWarn, logPrefixDNS, "rate limit exceeded for %s", clientIP)
		srv.sendDNSRefusal(w, req)
		return
	}

	srv.recordStats(func(s *ServerStats) { s.TotalQueries++ })

	question := req.Question[0]
	domain := strings.TrimSuffix(strings.ToLower(question.Name), ".")

	if cached, ok := srv.dnsCache.Get(domain); ok && question.Qtype == dns.TypeA {
		srv.Logf(LogLevelDebug, logPrefixDNS, "DNS cache HIT for %s", domain)
		srv.recordStats(func(s *ServerStats) { s.CacheHits++ })
		cached.SetReply(req)
		w.WriteMsg(cached)
		return
	}

	srv.recordStats(func(s *ServerStats) { s.CacheMisses++ })

	blocked, blockedReason, mlConfidence, err := srv.classifyDomain(domain, clientIP)

	if blocked {
		srv.Logf(LogLevelDebug, logPrefixDNS, "BLOCKED %s (reason: %s)", domain, blockedReason)
		srv.recordStats(func(s *ServerStats) { s.BlockedQueries++ })

		mlCategory := ""
		if strings.HasPrefix(blockedReason, "ml:") {
			mlCategory = strings.TrimPrefix(blockedReason, "ml:")
		}
		srv.recordQuery(domain, clientIP, question.Qtype, "blocked", blockedReason, true, mlCategory, mlConfidence)
		srv.sendDNSRefusal(w, req)
		return
	}

	srv.recordStats(func(s *ServerStats) { s.AllowedQueries++ })

	resp, err := srv.queryUpstreams(req)
	if err != nil {
		srv.Logf(LogLevelWarn, logPrefixDNS, "upstream query failed for %s: %v", domain, err)
		srv.recordStats(func(s *ServerStats) { s.UpstreamErrors++ })
		srv.sendDNSRefusal(w, req)
		return
	}

	if question.Qtype == dns.TypeA && resp != nil && len(resp.Answer) > 0 {
		ttl := uint32(DNSDefaultTTL)
		if rr := resp.Answer[0]; rr != nil {
			if rrHdr := rr.Header(); rrHdr != nil && rrHdr.Ttl > 0 && rrHdr.Ttl < MaxTTL {
				ttl = rrHdr.Ttl
			}
		}
		srv.dnsCache.Set(domain, resp)
		srv.Logf(LogLevelDebug, logPrefixDNS, "cached response for %s (TTL: %d)", domain, ttl)
	}

	srv.recordQuery(domain, clientIP, question.Qtype, "allowed", "", false, "", mlConfidence)

	resp.SetReply(req)
	w.WriteMsg(resp)
}

func (srv *GuardianServer) classifyDomain(domain, clientIP string) (bool, string, float32, error) {
	// Custom rules check
	blockedByRule, allowedByRule, ruleReason := srv.checkCustomRules(domain, clientIP)
	if allowedByRule {
		return false, "", 0, nil
	}
	if blockedByRule {
		return true, ruleReason, 0, nil
	}

	srv.blocklistMu.RLock()
	if _, blocked := srv.blocklist[domain]; blocked {
		srv.blocklistMu.RUnlock()
		srv.recordStats(func(s *ServerStats) { s.BlocklistMatches++ })
		return true, "blocklist", 0, nil
	}
	srv.blocklistMu.RUnlock()

	if srv.mlClient != nil && !config.IsMLAllowlisted(domain) {
		isMalicious, category, confidence, err := srv.classifyWithML(domain)
		if err == nil {
			if isMalicious {
				// Check if confidence meets threshold before blocking
				srv.mlThresholdMu.RLock()
				if float64(confidence) < srv.mlThreshold {
					srv.mlThresholdMu.RUnlock()
					return false, "", confidence, nil
				}
				srv.mlThresholdMu.RUnlock()

				srv.recordStats(func(s *ServerStats) { s.MLClassified++ })
				return true, "ml:" + category, confidence, nil
			}
			return false, "", confidence, nil
		}
		srv.Logf(LogLevelDebug, logPrefixML, "ML classification failed for %s: %v", domain, err)
	}

	return false, "", 0, nil
}

func (srv *GuardianServer) checkCustomRules(domain, clientIP string) (bool, bool, string) {
	var groupIDs []int
	if clientIP != "" {
		srv.dbMu.RLock()
		rows, err := srv.db.QueryContext(srv.ctx, "SELECT group_id FROM client_group_members WHERE client_ip = ?", clientIP)
		if err == nil {
			for rows.Next() {
				var id int
				if rows.Scan(&id) == nil {
					groupIDs = append(groupIDs, id)
				}
			}
			rows.Close()
		}
		srv.dbMu.RUnlock()
	}

	// We check specific to general: client, then group, then global
	keys := []string{}
	if clientIP != "" {
		keys = append(keys, "rules:client:"+clientIP)
	}
	for _, id := range groupIDs {
		keys = append(keys, fmt.Sprintf("rules:group:%d", id))
	}
	keys = append(keys, "rules:global")

	for _, key := range keys {
		srv.dbMu.RLock()
		var ruleStr string
		err := srv.db.QueryRowContext(srv.ctx, "SELECT value FROM settings WHERE key = ?", key).Scan(&ruleStr)
		srv.dbMu.RUnlock()

		if err == nil && ruleStr != "" {
			for _, line := range strings.Split(ruleStr, "\n") {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "!") || strings.HasPrefix(line, "#") {
					continue
				}

				// AdGuard/Adblock syntax handling
				isException := strings.HasPrefix(line, "@@")
				if isException {
					line = strings.TrimPrefix(line, "@@")
				}
				line = strings.TrimPrefix(line, "||")
				line = strings.TrimSuffix(line, "^")

				if domain == line || strings.HasSuffix(domain, "."+line) {
					if isException {
						return false, true, "allowed_by_rule" // exception allows the domain
					}
					scope := strings.SplitN(key, ":", 2)
					if len(scope) > 1 {
						return true, false, "custom_rule:" + scope[1]
					}
					return true, false, "custom_rule"
				}
			}
		}
	}

	return false, false, ""
}

func (srv *GuardianServer) classifyWithML(domain string) (bool, string, float32, error) {
	srv.mlCacheMu.RLock()
	if cached, ok := srv.mlCache.Get(domain); ok {
		if cached.ExpiresAt.After(time.Now()) {
			srv.mlCacheMu.RUnlock()
			return cached.IsMalicious, cached.Category, cached.Confidence, nil
		}
	}
	srv.mlCacheMu.RUnlock()

	if srv.mlClient == nil {
		return false, "", 0, errors.New("ML service not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	resp, err := srv.mlClient.PredictDomain(ctx, &pb.DomainRequest{Domain: domain})
	if err != nil {
		return false, "", 0, err
	}

	srv.Logf(LogLevelInfo, logPrefixML, "ML classification for %s: isMalicious=%v category=%s confidence=%.2f", domain, resp.IsMalicious, resp.Category, resp.ConfidenceScore)

	entry := cache.MLCacheEntry{
		IsMalicious: resp.IsMalicious,
		Category:    resp.Category,
		Confidence:  resp.ConfidenceScore,
		ExpiresAt:   time.Now().Add(config.MLCacheTTL),
	}
	srv.mlCacheMu.Lock()
	srv.mlCache.Set(domain, entry)
	srv.mlCacheMu.Unlock()

	return resp.IsMalicious, resp.Category, resp.ConfidenceScore, nil
}

func (srv *GuardianServer) queryUpstreams(req *dns.Msg) (*dns.Msg, error) {
	srv.upstreamsMu.RLock()
	upstreams := make([]string, len(srv.upstreams))
	copy(upstreams, srv.upstreams)
	srv.upstreamsMu.RUnlock()

	ctx, cancel := context.WithTimeout(context.Background(), DNSTimeout)
	defer cancel()

	var lastErr error
	for _, addr := range upstreams {
		resp, err := srv.singleUpstreamLookup(ctx, req, addr)
		if err == nil && resp != nil {
			return resp, nil
		}
		lastErr = err
		srv.Logf(LogLevelDebug, logPrefixDNS, "upstream %s failed: %v", addr, err)
	}

	return nil, lastErr
}

func (srv *GuardianServer) singleUpstreamLookup(ctx context.Context, req *dns.Msg, addr string) (*dns.Msg, error) {
	if !strings.Contains(addr, ":") {
		addr = net.JoinHostPort(addr, "53")
	}

	for _, network := range []string{"udp", "tcp"} {
		client := &dns.Client{
			Net:     network,
			Timeout: 3 * time.Second,
		}

		resp, _, err := client.ExchangeContext(ctx, req, addr)
		if err == nil && resp != nil {
			return resp, nil
		}
	}

	return nil, fmt.Errorf("all upstream methods failed for %s", addr)
}

// ════════════════════════════════════════════════════════════════════════════════
// ─ DNS utility functions ──────────────────────────────────────────────────────
// ════════════════════════════════════════════════════════════════════════════════

func (srv *GuardianServer) sendDNSRefusal(w dns.ResponseWriter, req *dns.Msg) {
	resp := new(dns.Msg)
	resp.SetRcode(req, dns.RcodeRefused)
	w.WriteMsg(resp)
}

func (srv *GuardianServer) checkRateLimit(clientIP string) bool {
	srv.rateLimitMu.Lock()
	defer srv.rateLimitMu.Unlock()

	if _, exists := srv.rateLimiters[clientIP]; !exists {
		srv.rateLimiters[clientIP] = &cache.TokenBucket{}
	}

	return srv.rateLimiters[clientIP].Allow()
}

// ════════════════════════════════════════════════════════════════════════════════
// ─ Blocklist management and reloading ─────────────────────────────────────────
// ════════════════════════════════════════════════════════════════════════════════

func (srv *GuardianServer) reloadAllSources() error {
	newBlocklist := make(map[string]bool)

	if err := srv.loadHostsFile(srv.blocklistFile, newBlocklist); err != nil {
		srv.Logf(LogLevelWarn, logPrefixGuardian, "failed to load hosts file %s: %v", srv.blocklistFile, err)
	} else {
		srv.Logf(LogLevelInfo, logPrefixGuardian, "loaded %d entries from hosts file", len(newBlocklist))
	}

	if err := srv.loadBlocklistSources(newBlocklist); err != nil {
		srv.Logf(LogLevelWarn, logPrefixGuardian, "failed to load blocklist sources: %v", err)
	}

	srv.blocklistMu.Lock()
	srv.blocklist = newBlocklist
	srv.blocklistMu.Unlock()

	srv.Logf(LogLevelInfo, logPrefixGuardian, "blocklist reloaded: %d total domains", len(newBlocklist))
	return nil
}

func (srv *GuardianServer) loadHostsFile(filepath string, blocklist map[string]bool) error {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return err
	}

	lines := bytes.Split(data, []byte("\n"))
	hostsRegex := regexp.MustCompile(`^(0\.0\.0\.0|127\.0\.0\.1)\s+([a-z0-9.-]+)`)

	for _, line := range lines {
		line = bytes.TrimSpace(line)
		if len(line) == 0 || bytes.HasPrefix(line, []byte("#")) {
			continue
		}

		matches := hostsRegex.FindSubmatch(line)
		if len(matches) >= 3 {
			domain := strings.ToLower(string(matches[2]))
			blocklist[domain] = true
		}
	}

	return nil
}

func (srv *GuardianServer) loadBlocklistSources(blocklist map[string]bool) error {
	srv.dbMu.RLock()
	defer srv.dbMu.RUnlock()

	rows, err := srv.db.QueryContext(srv.ctx, `
		SELECT url FROM blocklist_sources WHERE enabled = 1
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var url string
		if err := rows.Scan(&url); err != nil {
			continue
		}

		// Fetch the blocklist from the URL
		resp, err := http.Get(url)
		if err != nil {
			srv.Logf(LogLevelWarn, logPrefixGuardian, "failed to fetch blocklist from %s: %v", url, err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			srv.Logf(LogLevelWarn, logPrefixGuardian, "failed to fetch blocklist from %s: status %d", url, resp.StatusCode)
			continue
		}

		// Parse the blocklist content
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			srv.Logf(LogLevelWarn, logPrefixGuardian, "failed to read blocklist from %s: %v", url, err)
			continue
		}

		// Parse as hosts file format or AdGuard format
		scanner := bufio.NewScanner(strings.NewReader(string(data)))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			// Skip empty lines and comments
			if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
				continue
			}

			// Skip allowlist entries (@@)
			if strings.HasPrefix(line, "@@") {
				continue
			}

			// Parse AdGuard format: ||domain.com^
			if strings.HasPrefix(line, "||") {
				domain := strings.TrimPrefix(line, "||")
				domain = strings.TrimSuffix(domain, "^")
				domain = strings.TrimSpace(domain)
				domain = strings.ToLower(domain)
				if domain != "" {
					blocklist[domain] = true
				}
				continue
			}

			// Parse hosts file format: IP domain [domain2 domain3 ...]
			parts := strings.Fields(line)
			if len(parts) < 2 {
				continue
			}
			// Add all domains (skip the first part which is the IP)
			for i := 1; i < len(parts); i++ {
				domain := strings.ToLower(parts[i])
				blocklist[domain] = true
			}
		}

		if err := scanner.Err(); err != nil {
			srv.Logf(LogLevelWarn, logPrefixGuardian, "error parsing blocklist from %s: %v", url, err)
			continue
		}

		srv.Logf(LogLevelInfo, logPrefixGuardian, "loaded blocklist from %s", url)
	}

	return rows.Err()
}

// ════════════════════════════════════════════════════════════════════════════════
// ─ HTTP handlers - Blocklist management ───────────────────────────────────────
// ════════════════════════════════════════════════════════════════════════════════

func (srv *GuardianServer) registerBlocklistHandlers(mux *http.ServeMux, dev bool) {
	mux.HandleFunc("GET /api/blocklists", srv.handleGetBlocklists)
	mux.HandleFunc("POST /api/blocklists", srv.handleCreateBlocklist)
	mux.HandleFunc("PUT /api/blocklists/{id}", srv.handleUpdateBlocklist)
	mux.HandleFunc("DELETE /api/blocklists/{id}", srv.handleDeleteBlocklist)
	mux.HandleFunc("POST /api/blocklists/{id}/reload", srv.handleReloadBlocklist)
	mux.HandleFunc("GET /api/blocklist/stats", srv.handleBlocklistStats)
	mux.HandleFunc("GET /api/predefined-blocklists", srv.handlePredefinedBlocklists)
	mux.HandleFunc("GET /api/blocklist/predefined", srv.handlePredefinedBlocklists)
	mux.HandleFunc("GET /api/blocklist", srv.handleGetBlocklistURLs)
	mux.HandleFunc("GET /api/blocklist/sources", srv.handleGetBlocklistSources)
	mux.HandleFunc("POST /api/blocklist/sources", srv.handleCreateBlocklistSource)
	mux.HandleFunc("GET /api/blocklist/enabled", srv.handleGetEnabledBlocklists)
	mux.HandleFunc("POST /api/blocklist/sources/reload", srv.handleReloadBlocklistSources)
}

func (srv *GuardianServer) handleGetBlocklists(w http.ResponseWriter, r *http.Request) {
	if !srv.verifyAuth(w, r) {
		return
	}

	srv.dbMu.RLock()
	defer srv.dbMu.RUnlock()

	rows, err := srv.db.QueryContext(r.Context(), `
		SELECT id, name, url, last_updated, entry_count, enabled FROM blocklist_sources ORDER BY created_at DESC
	`)
	if err != nil {
		srv.httpError(w, "failed to query blocklists", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	blocklists := []map[string]interface{}{}
	for rows.Next() {
		var id, name, url string
		var lastUpdated sql.NullTime
		var entryCount int
		var enabled bool

		if err := rows.Scan(&id, &name, &url, &lastUpdated, &entryCount, &enabled); err != nil {
			continue
		}

		blocklists = append(blocklists, map[string]interface{}{
			"id":           id,
			"name":         name,
			"url":          url,
			"last_updated": lastUpdated.Time,
			"entry_count":  entryCount,
			"enabled":      enabled,
		})
	}

	srv.httpJSON(w, blocklists, http.StatusOK)
}

func (srv *GuardianServer) handleCreateBlocklist(w http.ResponseWriter, r *http.Request) {
	if !srv.verifyAuth(w, r) {
		return
	}

	var req struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		srv.httpError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	id := randomID()
	now := time.Now().Unix()

	srv.dbMu.Lock()
	defer srv.dbMu.Unlock()

	_, err := srv.db.ExecContext(r.Context(), `
		INSERT INTO blocklist_sources (id, name, url, enabled, created_at)
		VALUES (?, ?, ?, 1, ?)
	`, id, req.Name, req.URL, now)

	if err != nil {
		srv.httpError(w, "failed to create blocklist", http.StatusInternalServerError)
		return
	}

	srv.Logf(LogLevelInfo, logPrefixHTTP, "created blocklist %s (%s)", id, req.Name)
	srv.httpJSON(w, map[string]string{"id": id}, http.StatusCreated)
}

func (srv *GuardianServer) handleUpdateBlocklist(w http.ResponseWriter, r *http.Request) {
	if !srv.verifyAuth(w, r) {
		return
	}

	id := r.PathValue("id")
	if id == "" {
		srv.httpError(w, "missing blocklist id", http.StatusBadRequest)
		return
	}

	var req struct {
		Name    *string `json:"name"`
		URL     *string `json:"url"`
		Enabled *bool   `json:"enabled"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		srv.httpError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	srv.dbMu.Lock()
	defer srv.dbMu.Unlock()

	query := "UPDATE blocklist_sources SET "
	args := []interface{}{}
	updates := []string{}

	if req.Name != nil {
		updates = append(updates, "name = ?")
		args = append(args, *req.Name)
	}
	if req.URL != nil {
		updates = append(updates, "url = ?")
		args = append(args, *req.URL)
	}
	if req.Enabled != nil {
		updates = append(updates, "enabled = ?")
		args = append(args, *req.Enabled)
	}

	if len(updates) == 0 {
		srv.httpError(w, "no fields to update", http.StatusBadRequest)
		return
	}

	query += strings.Join(updates, ", ") + " WHERE id = ?"
	args = append(args, id)

	_, err := srv.db.ExecContext(r.Context(), query, args...)
	if err != nil {
		srv.httpError(w, "failed to update blocklist", http.StatusInternalServerError)
		return
	}

	srv.Logf(LogLevelInfo, logPrefixHTTP, "updated blocklist %s", id)
	srv.httpStatus(w, http.StatusOK)
}

func (srv *GuardianServer) handleDeleteBlocklist(w http.ResponseWriter, r *http.Request) {
	if !srv.verifyAuth(w, r) {
		return
	}

	id := r.PathValue("id")
	if id == "" {
		srv.httpError(w, "missing blocklist id", http.StatusBadRequest)
		return
	}

	srv.dbMu.Lock()
	defer srv.dbMu.Unlock()

	_, err := srv.db.ExecContext(r.Context(), "DELETE FROM blocklist_sources WHERE id = ?", id)
	if err != nil {
		srv.httpError(w, "failed to delete blocklist", http.StatusInternalServerError)
		return
	}

	srv.Logf(LogLevelInfo, logPrefixHTTP, "deleted blocklist %s", id)
	srv.httpStatus(w, http.StatusOK)
}

func (srv *GuardianServer) handleReloadBlocklist(w http.ResponseWriter, r *http.Request) {
	if !srv.verifyAuth(w, r) {
		return
	}

	if err := srv.reloadAllSources(); err != nil {
		srv.httpError(w, fmt.Sprintf("reload failed: %v", err), http.StatusInternalServerError)
		return
	}

	srv.Logf(LogLevelInfo, logPrefixHTTP, "blocklist reloaded")
	srv.httpStatus(w, http.StatusOK)
}

func (srv *GuardianServer) handleReloadBlocklistSources(w http.ResponseWriter, r *http.Request) {
	if !srv.verifyAuth(w, r) {
		return
	}

	if err := srv.reloadAllSources(); err != nil {
		srv.httpError(w, fmt.Sprintf("reload failed: %v", err), http.StatusInternalServerError)
		return
	}

	srv.Logf(LogLevelInfo, logPrefixHTTP, "blocklist sources reloaded")
	srv.httpStatus(w, http.StatusOK)
}

func (srv *GuardianServer) handleBlocklistStats(w http.ResponseWriter, r *http.Request) {
	if !srv.verifyAuth(w, r) {
		return
	}

	srv.blocklistMu.RLock()
	entryCount := len(srv.blocklist)
	srv.blocklistMu.RUnlock()

	srv.httpJSON(w, map[string]interface{}{
		"total_entries": entryCount,
	}, http.StatusOK)
}

func (srv *GuardianServer) handlePredefinedBlocklists(w http.ResponseWriter, r *http.Request) {
	srv.httpJSON(w, config.PredefinedBlocklists, http.StatusOK)
}

func (srv *GuardianServer) handleGetBlocklistURLs(w http.ResponseWriter, r *http.Request) {
	if !srv.verifyAuth(w, r) {
		return
	}

	srv.dbMu.RLock()
	defer srv.dbMu.RUnlock()

	rows, err := srv.db.QueryContext(r.Context(), `
		SELECT url FROM blocklist_sources WHERE enabled = 1 ORDER BY created_at DESC
	`)
	if err != nil {
		srv.httpError(w, "failed to query blocklists", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	urls := []string{}
	for rows.Next() {
		var url string
		if err := rows.Scan(&url); err != nil {
			continue
		}
		urls = append(urls, url)
	}

	srv.httpJSON(w, urls, http.StatusOK)
}

func (srv *GuardianServer) handleGetBlocklistSources(w http.ResponseWriter, r *http.Request) {
	if !srv.verifyAuth(w, r) {
		return
	}

	srv.dbMu.RLock()
	defer srv.dbMu.RUnlock()

	rows, err := srv.db.QueryContext(r.Context(), `
		SELECT id, name, url, enabled FROM blocklist_sources ORDER BY created_at DESC
	`)
	if err != nil {
		srv.httpError(w, "failed to query blocklists", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	sources := []map[string]interface{}{}
	for rows.Next() {
		var id, name, url string
		var enabled bool
		if err := rows.Scan(&id, &name, &url, &enabled); err != nil {
			continue
		}
		sources = append(sources, map[string]interface{}{
			"id":      id,
			"name":    name,
			"url":     url,
			"enabled": enabled,
		})
	}

	srv.httpJSON(w, sources, http.StatusOK)
}

func (srv *GuardianServer) handleCreateBlocklistSource(w http.ResponseWriter, r *http.Request) {
	if !srv.verifyAuth(w, r) {
		return
	}

	// Read body once
	body, _ := io.ReadAll(r.Body)

	// Try to decode as array first (for Settings page bulk update)
	var reqArray []struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	}

	if err := json.Unmarshal(body, &reqArray); err == nil && len(reqArray) > 0 {
		// It's an array - replace all sources
		srv.dbMu.Lock()
		defer srv.dbMu.Unlock()

		// Delete all existing sources
		srv.db.ExecContext(r.Context(), "DELETE FROM blocklist_sources")

		// Insert new sources
		now := time.Now().Unix()
		for _, item := range reqArray {
			id := randomID()
			srv.db.ExecContext(r.Context(), `
				INSERT INTO blocklist_sources (id, name, url, enabled, created_at)
				VALUES (?, ?, ?, 1, ?)
			`, id, item.Name, item.URL, now)
		}

		srv.Logf(LogLevelInfo, logPrefixHTTP, "updated %d blocklist sources", len(reqArray))
		srv.httpStatus(w, http.StatusOK)
		return
	}

	// Single object format
	var req struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	}

	if err := json.Unmarshal(body, &req); err != nil {
		srv.httpError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" || req.URL == "" {
		srv.httpError(w, "name and url are required", http.StatusBadRequest)
		return
	}

	id := randomID()
	now := time.Now().Unix()

	srv.dbMu.Lock()
	defer srv.dbMu.Unlock()

	_, err := srv.db.ExecContext(r.Context(), `
		INSERT INTO blocklist_sources (id, name, url, enabled, created_at)
		VALUES (?, ?, ?, 1, ?)
	`, id, req.Name, req.URL, now)

	if err != nil {
		srv.httpError(w, "failed to create blocklist source", http.StatusInternalServerError)
		return
	}

	srv.Logf(LogLevelInfo, logPrefixHTTP, "created blocklist source %s (%s)", id, req.Name)
	srv.httpJSON(w, map[string]string{"id": id}, http.StatusCreated)
}

func (srv *GuardianServer) handleGetEnabledBlocklists(w http.ResponseWriter, r *http.Request) {
	if !srv.verifyAuth(w, r) {
		return
	}

	srv.dbMu.RLock()
	defer srv.dbMu.RUnlock()

	rows, err := srv.db.QueryContext(r.Context(), `
		SELECT id, name FROM blocklist_sources WHERE enabled = 1 ORDER BY created_at DESC
	`)
	if err != nil {
		srv.httpError(w, "failed to query blocklists", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	blocklists := []map[string]string{}
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			continue
		}
		blocklists = append(blocklists, map[string]string{
			"id":   id,
			"name": name,
		})
	}

	srv.httpJSON(w, blocklists, http.StatusOK)
}

// ════════════════════════════════════════════════════════════════════════════════
// ─ HTTP handlers - Upstream management ────────────────────────────────────────
// ════════════════════════════════════════════════════════════════════════════════

func (srv *GuardianServer) registerUpstreamHandlers(mux *http.ServeMux, dev bool) {
	mux.HandleFunc("GET /api/upstreams", srv.handleGetUpstreams)
	mux.HandleFunc("PUT /api/upstreams", srv.handleUpdateUpstreams)
	mux.HandleFunc("POST /api/upstreams/test", srv.handleTestUpstream)
	mux.HandleFunc("GET /api/upstream", srv.handleGetUpstreams)
	mux.HandleFunc("POST /api/upstream", srv.handleUpdateUpstreams)
}

func (srv *GuardianServer) handleGetUpstreams(w http.ResponseWriter, r *http.Request) {
	if !srv.verifyAuth(w, r) {
		return
	}

	srv.upstreamsMu.RLock()
	upstreams := make([]string, len(srv.upstreams))
	copy(upstreams, srv.upstreams)
	srv.upstreamsMu.RUnlock()

	srv.httpJSON(w, map[string]interface{}{
		"servers": upstreams,
	}, http.StatusOK)
}

func (srv *GuardianServer) handleUpdateUpstreams(w http.ResponseWriter, r *http.Request) {
	if !srv.verifyAuth(w, r) {
		return
	}

	srv.Logf(LogLevelInfo, logPrefixHTTP, "handleUpdateUpstreams called")

	var req struct {
		Servers []string `json:"servers"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		srv.Logf(LogLevelWarn, logPrefixHTTP, "failed to decode upstream request: %v", err)
		srv.httpError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	srv.Logf(LogLevelInfo, logPrefixHTTP, "received servers: %v", req.Servers)

	if len(req.Servers) == 0 {
		srv.Logf(LogLevelWarn, logPrefixHTTP, "empty servers list provided")
		srv.httpError(w, "at least one upstream required", http.StatusBadRequest)
		return
	}

	srv.upstreamsMu.Lock()
	srv.upstreams = req.Servers
	srv.upstreamsMu.Unlock()

	srv.Logf(LogLevelInfo, logPrefixHTTP, "updated upstreams: %v", req.Servers)
	srv.httpStatus(w, http.StatusOK)
}

func (srv *GuardianServer) handleTestUpstream(w http.ResponseWriter, r *http.Request) {
	if !srv.verifyAuth(w, r) {
		return
	}

	var req struct {
		Address string `json:"address"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		srv.httpError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	msg := new(dns.Msg)
	msg.SetQuestion("google.com.", dns.TypeA)

	_, _, err := (&dns.Client{Timeout: 3 * time.Second}).Exchange(msg, req.Address)
	if err != nil {
		srv.httpJSON(w, map[string]interface{}{
			"status": "failed",
			"error":  err.Error(),
		}, http.StatusOK)
		return
	}

	srv.httpJSON(w, map[string]interface{}{
		"status": "ok",
	}, http.StatusOK)
}

// ════════════════════════════════════════════════════════════════════════════════
// ─ HTTP handlers - ML feedback and classification ─────────────────────────────
// ════════════════════════════════════════════════════════════════════════════════

func (srv *GuardianServer) registerMLHandlers(mux *http.ServeMux, dev bool) {
	mux.HandleFunc("GET /api/ml/classify", srv.handleMLClassify)
	mux.HandleFunc("POST /api/ml/feedback", srv.handleMLFeedback)
	mux.HandleFunc("GET /api/ml/cache-stats", srv.handleMLCacheStats)
	mux.HandleFunc("DELETE /api/ml/cache", srv.handleClearMLCache)
	mux.HandleFunc("GET /api/ml/settings", srv.handleGetMLSettings)
	mux.HandleFunc("POST /api/ml/settings", srv.handleSetMLSettings)
}

func (srv *GuardianServer) handleMLClassify(w http.ResponseWriter, r *http.Request) {
	if !srv.verifyAuth(w, r) {
		return
	}

	domain := strings.TrimSuffix(strings.ToLower(r.URL.Query().Get("domain")), ".")
	if domain == "" {
		srv.httpError(w, "missing domain parameter", http.StatusBadRequest)
		return
	}

	isMalicious, category, confidence, err := srv.classifyWithML(domain)
	if err != nil {
		srv.httpError(w, fmt.Sprintf("classification failed: %v", err), http.StatusInternalServerError)
		return
	}

	srv.httpJSON(w, map[string]interface{}{
		"domain":       domain,
		"is_malicious": isMalicious,
		"category":     category,
		"confidence":   confidence,
	}, http.StatusOK)
}

func (srv *GuardianServer) handleMLFeedback(w http.ResponseWriter, r *http.Request) {
	if !srv.verifyAuth(w, r) {
		return
	}

	var req struct {
		Domain         string  `json:"domain"`
		Verdict        string  `json:"verdict"`
		UserConfidence float32 `json:"user_confidence"`
		DomainQueryID  string  `json:"domain_query_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		srv.httpError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Domain == "" {
		srv.httpError(w, "domain required", http.StatusBadRequest)
		return
	}

	verdictMap := map[string]bool{"correct": true, "false_positive": true, "false_negative": true}
	if !verdictMap[req.Verdict] {
		srv.httpError(w, "invalid verdict", http.StatusBadRequest)
		return
	}

	srv.dbMu.Lock()
	defer srv.dbMu.Unlock()

	now := time.Now().Unix()

	if req.DomainQueryID != "" {
		_, err := srv.db.ExecContext(r.Context(), `
			UPDATE queries
			SET feedback_verdict = ?, feedback_confidence = ?, feedback_timestamp = ?
			WHERE id = ?
		`, req.Verdict, req.UserConfidence, now, req.DomainQueryID)

		if err != nil {
			srv.httpError(w, "failed to record feedback", http.StatusInternalServerError)
			return
		}
	} else {
		id := randomID()
		_, err := srv.db.ExecContext(r.Context(), `
			INSERT INTO queries (id, domain, feedback_verdict, feedback_confidence, feedback_timestamp, created_at)
			VALUES (?, ?, ?, ?, ?, ?)
		`, id, req.Domain, req.Verdict, req.UserConfidence, now, now)

		if err != nil {
			srv.httpError(w, "failed to record feedback", http.StatusInternalServerError)
			return
		}
	}

	// Update in-memory feedback statistics
	srv.feedbackMu.Lock()
	srv.feedbackTotal++
	if req.Verdict == "correct" || req.Verdict == "false_positive" {
		srv.feedbackSafe++
	} else if req.Verdict == "false_negative" {
		srv.feedbackMal++
	}
	srv.feedbackMu.Unlock()

	srv.Logf(LogLevelInfo, logPrefixML, "feedback recorded for %s: %s (confidence: %f)", req.Domain, req.Verdict, req.UserConfidence)
	srv.httpStatus(w, http.StatusOK)
}

func (srv *GuardianServer) handleMLCacheStats(w http.ResponseWriter, r *http.Request) {
	if !srv.verifyAuth(w, r) {
		return
	}

	srv.mlCacheMu.RLock()
	snapshot := srv.mlCache.Snapshot()
	srv.mlCacheMu.RUnlock()

	srv.httpJSON(w, map[string]interface{}{
		"cached_entries": len(snapshot),
		"max_size":       config.MaxMLCacheSize,
	}, http.StatusOK)
}

func (srv *GuardianServer) handleClearMLCache(w http.ResponseWriter, r *http.Request) {
	if !srv.verifyAuth(w, r) {
		return
	}

	srv.mlCacheMu.Lock()
	srv.mlCache = cache.NewLRUCache[string, cache.MLCacheEntry](config.MaxMLCacheSize)
	srv.mlCacheMu.Unlock()

	srv.Logf(LogLevelInfo, logPrefixHTTP, "ML cache cleared")
	srv.httpStatus(w, http.StatusOK)
}

func (srv *GuardianServer) handleGetMLSettings(w http.ResponseWriter, r *http.Request) {
	if !srv.verifyAuth(w, r) {
		return
	}

	// Get feedback statistics from memory
	srv.feedbackMu.RLock()
	feedbackTotal := srv.feedbackTotal
	feedbackSafe := srv.feedbackSafe
	feedbackMal := srv.feedbackMal
	srv.feedbackMu.RUnlock()

	// Get ML cache size
	srv.mlCacheMu.RLock()
	cacheSnapshot := srv.mlCache.Snapshot()
	srv.mlCacheMu.RUnlock()

	// Get ML threshold from memory
	srv.mlThresholdMu.RLock()
	threshold := srv.mlThreshold
	srv.mlThresholdMu.RUnlock()

	srv.httpJSON(w, map[string]interface{}{
		"enabled":        true,
		"model":          "default",
		"confidence":     threshold,
		"cache_enabled":  true,
		"threshold":      threshold,
		"block_dga":      true,
		"block_phishing": true,
		"block_malware":  true,
		"block_other":    false,
		"cache_size":     len(cacheSnapshot),
		"cache_max":      10000,
		"cache_ttl_min":  300,
		"feedback_total": feedbackTotal,
		"feedback_safe":  feedbackSafe,
		"feedback_mal":   feedbackMal,
		"ml_connected":   srv.mlClient != nil,
	}, http.StatusOK)
}

func (srv *GuardianServer) handleSetMLSettings(w http.ResponseWriter, r *http.Request) {
	if !srv.verifyAuth(w, r) {
		return
	}

	var req struct {
		Enabled      *bool    `json:"enabled"`
		Model        *string  `json:"model"`
		Threshold    *float64 `json:"threshold"`
		CacheEnabled *bool    `json:"cache_enabled"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		srv.httpError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Update ML threshold if provided
	if req.Threshold != nil {
		srv.mlThresholdMu.Lock()
		srv.mlThreshold = *req.Threshold
		srv.mlThresholdMu.Unlock()

		// Persist to settings table
		thresholdStr := fmt.Sprintf("%.2f", *req.Threshold)
		if err := srv.setSetting("ml_threshold", thresholdStr); err != nil {
			srv.Logf(LogLevelError, logPrefixML, "failed to save ML threshold to database: %v", err)
			srv.httpError(w, "failed to save threshold", http.StatusInternalServerError)
			return
		}
		srv.Logf(LogLevelInfo, logPrefixML, "ML threshold updated to %.2f and persisted to database", *req.Threshold)
	}

	srv.Logf(LogLevelInfo, logPrefixHTTP, "ML settings updated")
	srv.httpJSON(w, map[string]string{"status": "ok"}, http.StatusOK)
}

// ════════════════════════════════════════════════════════════════════════════════
// ─ HTTP handlers - Authentication ────────────────────────────────────────────
// ════════════════════════════════════════════════════════════════════════════════

func (srv *GuardianServer) registerAuthHandlers(mux *http.ServeMux, dev bool) {
	mux.HandleFunc("GET /api/version", srv.handleGetVersion)
	mux.HandleFunc("GET /api/setup-needed", srv.handleSetupNeeded)
	mux.HandleFunc("POST /api/auth/login", srv.handleLogin)
	mux.HandleFunc("POST /api/auth/logout", srv.handleLogout)
	mux.HandleFunc("POST /api/login", srv.handleLogin)   // Fallback for non-auth path
	mux.HandleFunc("POST /api/logout", srv.handleLogout) // Fallback for non-auth path
	mux.HandleFunc("GET /api/auth/session", srv.handleGetSession)
	mux.HandleFunc("GET /api/user", srv.handleGetUser)
	mux.HandleFunc("POST /api/auth/setup", srv.handleSetupDefault)
	mux.HandleFunc("GET /api/auth/current-ip", srv.handleCurrentIP)
}

func (srv *GuardianServer) handleGetVersion(w http.ResponseWriter, r *http.Request) {
	response := map[string]string{
		"version": srv.appVersion,
	}
	srv.httpJSON(w, response, 200)
}

func (srv *GuardianServer) handleSetupNeeded(w http.ResponseWriter, r *http.Request) {
	srv.dbMu.RLock()
	defer srv.dbMu.RUnlock()

	var count int
	err := srv.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM users").Scan(&count)
	if err != nil {
		srv.httpJSON(w, map[string]bool{"needed": true}, http.StatusOK)
		return
	}

	srv.httpJSON(w, map[string]bool{"needed": count == 0}, http.StatusOK)
}

func (srv *GuardianServer) handleCurrentIP(w http.ResponseWriter, r *http.Request) {
	// Try to get client IP from headers or remote address
	ip := r.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip = r.Header.Get("X-Real-IP")
	}
	if ip == "" {
		// Use net.SplitHostPort to handle both IPv4 and IPv6
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		ip = host
	}

	// Strip brackets from IPv6 addresses
	ip = strings.TrimPrefix(ip, "[")
	ip = strings.TrimSuffix(ip, "]")

	// If localhost or empty, use 127.0.0.1
	if ip == "127.0.0.1" || ip == "localhost" || ip == "" || ip == "::1" {
		ip = "127.0.0.1"
	}

	srv.httpJSON(w, map[string]string{"ip": ip}, http.StatusOK)
}

func (srv *GuardianServer) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		srv.httpError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Username == "" || req.Password == "" {
		srv.httpError(w, "username and password required", http.StatusBadRequest)
		return
	}

	srv.dbMu.Lock()
	defer srv.dbMu.Unlock()

	var userID string
	var isAdmin bool
	var passwordHash string

	err := srv.db.QueryRowContext(r.Context(), `
		SELECT id, is_admin, password_hash FROM users WHERE username = ?
	`, req.Username).Scan(&userID, &isAdmin, &passwordHash)

	if err != nil || err == sql.ErrNoRows {
		srv.httpError(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	if !verifyPassword(passwordHash, req.Password) {
		srv.httpError(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	sessionID := randomID()
	isAdmin = true
	expiresAt := time.Now().Add(config.SessionTTL)

	_, err = srv.db.ExecContext(r.Context(), `
		UPDATE users SET last_login = ? WHERE id = ?
	`, time.Now().Unix(), userID)

	_, err = srv.db.ExecContext(r.Context(), `
		INSERT INTO sessions (id, user_id, username, is_admin, expires_at)
		VALUES (?, ?, ?, ?, ?)
	`, sessionID, userID, req.Username, isAdmin, expiresAt.Format(time.RFC3339))

	if err != nil {
		srv.httpError(w, "failed to create session", http.StatusInternalServerError)
		return
	}

	srv.sessionMu.Lock()
	srv.sessions[sessionID] = &UserSession{
		UserID:    userID,
		Username:  req.Username,
		IsAdmin:   isAdmin,
		CreatedAt: time.Now(),
		ExpiresAt: expiresAt,
	}
	srv.sessionMu.Unlock()

	http.SetCookie(w, &http.Cookie{
		Name:     config.SessionCookieName,
		Value:    sessionID,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	srv.Logf(LogLevelInfo, logPrefixAuth, "user %s logged in", req.Username)
	srv.httpJSON(w, map[string]string{"session_id": sessionID}, http.StatusOK)
}

func (srv *GuardianServer) handleLogout(w http.ResponseWriter, r *http.Request) {
	sessionID, err := r.Cookie(config.SessionCookieName)
	if err != nil {
		srv.httpError(w, "not logged in", http.StatusUnauthorized)
		return
	}

	srv.sessionMu.Lock()
	delete(srv.sessions, sessionID.Value)
	srv.sessionMu.Unlock()

	srv.dbMu.Lock()
	srv.db.ExecContext(r.Context(), "DELETE FROM sessions WHERE id = ?", sessionID.Value)
	srv.dbMu.Unlock()

	http.SetCookie(w, &http.Cookie{
		Name:     config.SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})

	srv.Logf(LogLevelInfo, logPrefixAuth, "user logged out")
	srv.httpStatus(w, http.StatusOK)
}

func (srv *GuardianServer) handleGetSession(w http.ResponseWriter, r *http.Request) {
	sessionID, err := r.Cookie(config.SessionCookieName)
	if err != nil {
		srv.httpError(w, "not logged in", http.StatusUnauthorized)
		return
	}

	srv.sessionMu.RLock()
	session, exists := srv.sessions[sessionID.Value]
	srv.sessionMu.RUnlock()

	// If not in memory, check database
	if !exists {
		srv.dbMu.RLock()
		var username string
		var isAdmin bool
		var expiresAtStr string
		err := srv.db.QueryRowContext(r.Context(), `
			SELECT username, is_admin, expires_at FROM sessions WHERE id = ?
		`, sessionID.Value).Scan(&username, &isAdmin, &expiresAtStr)
		srv.dbMu.RUnlock()

		expiresAt, parseErr := parseSessionTimestamp(expiresAtStr)
		if err != nil || parseErr != nil || expiresAt.Before(time.Now()) {
			srv.httpError(w, "session expired", http.StatusUnauthorized)
			return
		}

		// Repopulate in-memory cache
		srv.sessionMu.Lock()
		srv.sessions[sessionID.Value] = &UserSession{
			UserID:    "",
			Username:  username,
			IsAdmin:   isAdmin,
			CreatedAt: time.Now(),
			ExpiresAt: expiresAt,
		}
		srv.sessionMu.Unlock()

		srv.httpJSON(w, map[string]interface{}{
			"username":   username,
			"is_admin":   isAdmin,
			"expires_at": expiresAt,
		}, http.StatusOK)
		return
	}

	if session.ExpiresAt.Before(time.Now()) {
		srv.httpError(w, "session expired", http.StatusUnauthorized)
		return
	}

	srv.httpJSON(w, map[string]interface{}{
		"username":   session.Username,
		"is_admin":   session.IsAdmin,
		"expires_at": session.ExpiresAt,
	}, http.StatusOK)
}

func (srv *GuardianServer) handleGetUser(w http.ResponseWriter, r *http.Request) {
	sessionID, err := r.Cookie(config.SessionCookieName)
	if err != nil {
		srv.httpError(w, "not logged in", http.StatusUnauthorized)
		return
	}

	srv.sessionMu.RLock()
	session, exists := srv.sessions[sessionID.Value]
	srv.sessionMu.RUnlock()

	// If not in memory, check database
	if !exists {
		srv.dbMu.RLock()
		var username string
		var isAdmin bool
		var expiresAtStr string
		err := srv.db.QueryRowContext(r.Context(), `
			SELECT username, is_admin, expires_at FROM sessions WHERE id = ?
		`, sessionID.Value).Scan(&username, &isAdmin, &expiresAtStr)
		srv.dbMu.RUnlock()

		expiresAt, parseErr := parseSessionTimestamp(expiresAtStr)
		if err != nil || parseErr != nil || expiresAt.Before(time.Now()) {
			srv.httpError(w, "not logged in", http.StatusUnauthorized)
			return
		}

		// Repopulate in-memory cache
		srv.sessionMu.Lock()
		srv.sessions[sessionID.Value] = &UserSession{
			UserID:    "",
			Username:  username,
			IsAdmin:   isAdmin,
			CreatedAt: time.Now(),
			ExpiresAt: expiresAt,
		}
		srv.sessionMu.Unlock()

		srv.httpJSON(w, map[string]string{
			"username": username,
		}, http.StatusOK)
		return
	}

	if session.ExpiresAt.Before(time.Now()) {
		srv.httpError(w, "not logged in", http.StatusUnauthorized)
		return
	}

	srv.httpJSON(w, map[string]string{
		"username": session.Username,
	}, http.StatusOK)
}

func (srv *GuardianServer) handleSetupDefault(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		srv.httpError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Username == "" || req.Password == "" {
		srv.httpError(w, "username and password required", http.StatusBadRequest)
		return
	}

	srv.dbMu.Lock()
	defer srv.dbMu.Unlock()

	var count int
	err := srv.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM users").Scan(&count)
	if err != nil || count > 0 {
		srv.httpError(w, "users already configured", http.StatusConflict)
		return
	}

	userID := randomID()
	passwordHash := hashPassword(req.Password)

	_, err = srv.db.ExecContext(r.Context(), `
		INSERT INTO users (id, username, password_hash, is_admin, created_at)
		VALUES (?, ?, ?, 1, ?)
	`, userID, req.Username, passwordHash, time.Now().Unix())

	if err != nil {
		srv.httpError(w, "failed to create user", http.StatusInternalServerError)
		return
	}

	// Create session for immediate login
	sessionID := randomID()
	isAdmin := true
	expiresAt := time.Now().Add(config.SessionTTL)

	_, err = srv.db.ExecContext(r.Context(), `
		INSERT INTO sessions (id, user_id, username, is_admin, expires_at)
		VALUES (?, ?, ?, ?, ?)
	`, sessionID, userID, req.Username, isAdmin, expiresAt.Format(time.RFC3339))

	if err != nil {
		srv.httpError(w, "failed to create session", http.StatusInternalServerError)
		return
	}

	// Add default blocklists during setup
	defaultBlocklists := []map[string]string{
		{"name": "AdGuard DNS filter", "url": "https://adguardteam.github.io/HostlistsRegistry/assets/filter_1.txt"},
		{"name": "Steven Black's List", "url": "https://adguardteam.github.io/HostlistsRegistry/assets/filter_33.txt"},
		{"name": "OISD Blocklist Small", "url": "https://adguardteam.github.io/HostlistsRegistry/assets/filter_5.txt"},
	}

	for _, bl := range defaultBlocklists {
		blID := randomID()
		_, err = srv.db.ExecContext(r.Context(), `
			INSERT INTO blocklist_sources (id, name, url, enabled)
			VALUES (?, ?, ?, 1)
		`, blID, bl["name"], bl["url"])
		if err != nil {
			srv.Logf(LogLevelWarn, logPrefixAuth, "failed to add default blocklist %s: %v", bl["name"], err)
		}
	}

	srv.sessionMu.Lock()
	srv.sessions[sessionID] = &UserSession{
		UserID:    userID,
		Username:  req.Username,
		IsAdmin:   isAdmin,
		CreatedAt: time.Now(),
		ExpiresAt: expiresAt,
	}
	srv.sessionMu.Unlock()

	http.SetCookie(w, &http.Cookie{
		Name:     config.SessionCookieName,
		Value:    sessionID,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	srv.Logf(LogLevelInfo, logPrefixAuth, "default admin user created and logged in: %s", req.Username)
	srv.httpJSON(w, map[string]string{"session_id": sessionID}, http.StatusOK)
}

// ════════════════════════════════════════════════════════════════════════════════
// ─ HTTP handlers - Queries and statistics ─────────────────────────────────────
// ════════════════════════════════════════════════════════════════════════════════

func (srv *GuardianServer) registerQueryHandlers(mux *http.ServeMux, dev bool) {
	mux.HandleFunc("GET /api/queries", srv.handleGetQueries)
	mux.HandleFunc("GET /api/queries/{id}", srv.handleGetQuery)
	mux.HandleFunc("GET /api/stats", srv.handleGetStats)
	mux.HandleFunc("DELETE /api/queries", srv.handleClearQueries)
	mux.HandleFunc("POST /api/queries/allow", srv.handleAllowQuery)
	mux.HandleFunc("POST /api/queries/block", srv.handleBlockQuery)
	mux.HandleFunc("GET /api/test-domain", srv.handleTestDomain)
}

func (srv *GuardianServer) handleGetQueries(w http.ResponseWriter, r *http.Request) {
	if !srv.verifyAuth(w, r) {
		return
	}

	limit := 1000
	limitStr := r.URL.Query().Get("limit")
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 10000 {
			limit = l
		}
	}

	offset := 0
	offsetStr := r.URL.Query().Get("offset")
	if offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	blocked := r.URL.Query().Get("blocked")
	qtype := r.URL.Query().Get("type")
	query := r.URL.Query().Get("q")

	srv.dbMu.RLock()
	defer srv.dbMu.RUnlock()

	// Build WHERE clause
	whereClause := ""
	args := []interface{}{}

	if blocked == "1" {
		whereClause = "WHERE blocked_by != ''"
	}
	if qtype != "" && qtype != "ALL" {
		if whereClause != "" {
			whereClause += " AND query_type = ?"
		} else {
			whereClause = "WHERE query_type = ?"
		}
		args = append(args, qtype)
	}
	if query != "" {
		if whereClause != "" {
			whereClause += " AND domain LIKE ?"
		} else {
			whereClause = "WHERE domain LIKE ?"
		}
		args = append(args, "%"+query+"%")
	}

	// Get total count
	countQuery := "SELECT COUNT(*) FROM queries " + whereClause
	var total int
	_ = srv.db.QueryRowContext(r.Context(), countQuery, args...).Scan(&total)

	// Get paginated results
	args = append(args, limit, offset)
	rows, err := srv.db.QueryContext(r.Context(), `
		SELECT id, domain, client_ip, query_type, result, blocked_by, is_malicious, ml_category, ml_confidence, response_time_ms, created_at
		FROM queries
		`+whereClause+`
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`, args...)

	if err != nil {
		srv.httpError(w, "failed to query database", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	queries := make([]map[string]interface{}, 0)
	for rows.Next() {
		var id, domain, clientIP, queryType, result, blockedBy string
		var isMalicious sql.NullBool
		var mlCategory sql.NullString
		var mlConfidence sql.NullFloat64
		var responseTime sql.NullFloat64
		var createdAt sql.NullInt64
		createdAtStr := ""

		if err := rows.Scan(&id, &domain, &clientIP, &queryType, &result, &blockedBy, &isMalicious, &mlCategory, &mlConfidence, &responseTime, &createdAt); err != nil {
			continue
		}

		if createdAt.Valid {
			createdAtStr = time.Unix(createdAt.Int64, 0).Format(time.RFC3339)
		} else {
			createdAtStr = time.Now().Format(time.RFC3339)
		}

		// Determine if blocked and category
		isBlocked := blockedBy != ""
		category := ""
		confidence := 0.0

		if mlCategory.Valid {
			category = mlCategory.String
		}
		if mlConfidence.Valid {
			confidence = mlConfidence.Float64
		}
		if blockedBy != "" && category == "" {
			category = blockedBy
		}

		reason := category
		if isBlocked && reason == "" {
			reason = "blocklist"
		}

		queries = append(queries, map[string]interface{}{
			"id":               id,
			"domain":           domain,
			"client_ip":        clientIP,
			"qtype":            queryType,
			"result":           result,
			"blocked_by":       blockedBy,
			"blocked":          isBlocked,
			"is_malicious":     isMalicious.Bool,
			"category":         category,
			"confidence":       confidence,
			"response_time_ms": responseTime.Float64,
			"reason":           reason,
			"timestamp":        createdAtStr,
		})
	}

	w.Header().Set("X-Total-Count", fmt.Sprintf("%d", total))
	srv.httpJSON(w, queries, http.StatusOK)
}

func (srv *GuardianServer) handleGetQuery(w http.ResponseWriter, r *http.Request) {
	if !srv.verifyAuth(w, r) {
		return
	}

	id := r.PathValue("id")
	if id == "" {
		srv.httpError(w, "missing query id", http.StatusBadRequest)
		return
	}

	srv.dbMu.RLock()
	defer srv.dbMu.RUnlock()

	var domain, clientIP, result, blockedBy string
	var isMalicious sql.NullBool
	var responseTime sql.NullFloat64
	var createdAt sql.NullInt64
	createdAtStr := ""

	err := srv.db.QueryRowContext(r.Context(), `
		SELECT domain, client_ip, result, blocked_by, is_malicious, response_time_ms, created_at
		FROM queries WHERE id = ?
	`, id).Scan(&domain, &clientIP, &result, &blockedBy, &isMalicious, &responseTime, &createdAt)

	if err == sql.ErrNoRows {
		srv.httpError(w, "query not found", http.StatusNotFound)
		return
	}
	if err != nil {
		srv.httpError(w, "failed to query database", http.StatusInternalServerError)
		return
	}

	if createdAt.Valid {
		createdAtStr = time.Unix(createdAt.Int64, 0).Format(time.RFC3339)
	} else {
		createdAtStr = time.Now().Format(time.RFC3339)
	}

	srv.httpJSON(w, map[string]interface{}{
		"id":               id,
		"domain":           domain,
		"client_ip":        clientIP,
		"result":           result,
		"blocked_by":       blockedBy,
		"is_malicious":     isMalicious.Bool,
		"response_time_ms": responseTime.Float64,
		"timestamp":        createdAtStr,
	}, http.StatusOK)
}

func (srv *GuardianServer) handleGetStats(w http.ResponseWriter, r *http.Request) {
	if !srv.verifyAuth(w, r) {
		return
	}

	srv.dbMu.RLock()
	defer srv.dbMu.RUnlock()

	// Get basic stats
	var total, blocked, mlBlocked int64
	var total24h, blocked24h int64

	_ = srv.db.QueryRowContext(r.Context(), `
		SELECT COUNT(*) FROM queries
	`).Scan(&total)

	_ = srv.db.QueryRowContext(r.Context(), `
		SELECT COUNT(*) FROM queries WHERE blocked_by != ''
	`).Scan(&blocked)

	_ = srv.db.QueryRowContext(r.Context(), `
		SELECT COUNT(*) FROM queries WHERE blocked_by != '' AND ml_category LIKE '%ml%'
	`).Scan(&mlBlocked)

	cutoff := time.Now().Add(-24 * time.Hour).Unix()
	_ = srv.db.QueryRowContext(r.Context(), `
		SELECT COUNT(*) FROM queries WHERE created_at > ?
	`, cutoff).Scan(&total24h)

	_ = srv.db.QueryRowContext(r.Context(), `
		SELECT COUNT(*) FROM queries WHERE blocked_by != '' AND created_at > ?
	`, cutoff).Scan(&blocked24h)

	// Get top domains
	topDomains := []map[string]interface{}{}
	rows, err := srv.db.QueryContext(r.Context(), `
		SELECT domain, COUNT(*) as cnt FROM queries GROUP BY domain ORDER BY cnt DESC LIMIT 10
	`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var domain string
			var count int
			if err := rows.Scan(&domain, &count); err == nil {
				topDomains = append(topDomains, map[string]interface{}{"domain": domain, "count": count})
			}
		}
	}
	if topDomains == nil {
		topDomains = []map[string]interface{}{}
	}

	// Get top blocked
	topBlocked := []map[string]interface{}{}
	rows, err = srv.db.QueryContext(r.Context(), `
		SELECT domain, COUNT(*) as cnt FROM queries WHERE blocked_by != '' GROUP BY domain ORDER BY cnt DESC LIMIT 10
	`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var domain string
			var count int
			if err := rows.Scan(&domain, &count); err == nil {
				topBlocked = append(topBlocked, map[string]interface{}{"domain": domain, "count": count})
			}
		}
	}
	if topBlocked == nil {
		topBlocked = []map[string]interface{}{}
	}

	// Get qtype breakdown
	qtypeBreakdown := []map[string]interface{}{}
	rows, err = srv.db.QueryContext(r.Context(), `
		SELECT query_type, COUNT(*) as cnt FROM queries GROUP BY query_type ORDER BY cnt DESC LIMIT 10
	`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var queryType string
			var count int
			if err := rows.Scan(&queryType, &count); err == nil {
				label := queryType
				if queryType == "" {
					label = "UNKNOWN"
				}
				qtypeBreakdown = append(qtypeBreakdown, map[string]interface{}{"qtype": queryType, "label": label, "count": count})
			}
		}
	}
	if qtypeBreakdown == nil {
		qtypeBreakdown = []map[string]interface{}{}
	}

	// Get category breakdown
	catBreakdown := []map[string]interface{}{}
	rows, err = srv.db.QueryContext(r.Context(), `
		SELECT ml_category, COUNT(*) as cnt FROM queries WHERE blocked_by != '' GROUP BY ml_category ORDER BY cnt DESC LIMIT 10
	`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var category string
			var count int
			if err := rows.Scan(&category, &count); err == nil {
				catBreakdown = append(catBreakdown, map[string]interface{}{"category": category, "count": count})
			}
		}
	}
	if catBreakdown == nil {
		catBreakdown = []map[string]interface{}{}
	}

	// Get block reasons breakdown
	blockReasons := []map[string]interface{}{}
	rows, err = srv.db.QueryContext(r.Context(), `
		SELECT blocked_by, COUNT(*) as cnt FROM queries WHERE blocked_by != '' GROUP BY blocked_by ORDER BY cnt DESC LIMIT 10
	`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var reason string
			var count int
			if err := rows.Scan(&reason, &count); err == nil {
				blockReasons = append(blockReasons, map[string]interface{}{"reason": reason, "count": count})
			}
		}
	}
	if blockReasons == nil {
		blockReasons = []map[string]interface{}{}
	}

	// Check ML connection status
	mlConnected := srv.mlClient != nil

	response := map[string]interface{}{
		"total":           total,
		"blocked":         blocked,
		"ml_blocked":      mlBlocked,
		"total_24h":       total24h,
		"blocked_24h":     blocked24h,
		"top_domains":     topDomains,
		"top_blocked":     topBlocked,
		"qtype_breakdown": qtypeBreakdown,
		"cat_breakdown":   catBreakdown,
		"block_reasons":   blockReasons,
		"ml_enabled":      true,
		"ml_connected":    mlConnected,
	}

	srv.httpJSON(w, response, http.StatusOK)
}

func (srv *GuardianServer) handleClearQueries(w http.ResponseWriter, r *http.Request) {
	if !srv.verifyAuth(w, r) {
		return
	}

	srv.dbMu.Lock()
	defer srv.dbMu.Unlock()

	_, err := srv.db.ExecContext(r.Context(), "DELETE FROM queries")
	if err != nil {
		srv.httpError(w, "failed to clear queries", http.StatusInternalServerError)
		return
	}

	srv.Logf(LogLevelInfo, logPrefixHTTP, "query logs cleared")
	srv.httpStatus(w, http.StatusOK)
}

func (srv *GuardianServer) handleAllowQuery(w http.ResponseWriter, r *http.Request) {
	if !srv.verifyAuth(w, r) {
		return
	}

	var req struct {
		Domain string `json:"domain"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		srv.httpError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	domain := strings.TrimSpace(strings.ToLower(req.Domain))
	if domain == "" {
		srv.httpError(w, "domain required", http.StatusBadRequest)
		return
	}

	ruleID := randomID()
	now := time.Now().Unix()

	srv.dbMu.Lock()
	defer srv.dbMu.Unlock()

	_, err := srv.db.ExecContext(r.Context(), `
		INSERT INTO rules (id, name, condition_type, condition_value, action, priority, enabled, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, ruleID, fmt.Sprintf("Quick allow: %s", domain), "domain", domain, "allow", 1, true, now)

	if err != nil {
		srv.Logf(LogLevelWarn, logPrefixHTTP, "failed to create allow rule for %s: %v", domain, err)
		srv.httpError(w, "failed to create allow rule", http.StatusInternalServerError)
		return
	}

	srv.Logf(LogLevelInfo, logPrefixHTTP, "query allowed: %s (rule: %s)", domain, ruleID)
	srv.httpJSON(w, map[string]string{"status": "ok"}, http.StatusOK)
}

func (srv *GuardianServer) handleBlockQuery(w http.ResponseWriter, r *http.Request) {
	if !srv.verifyAuth(w, r) {
		return
	}

	var req struct {
		Domain string `json:"domain"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		srv.httpError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	domain := strings.TrimSpace(strings.ToLower(req.Domain))
	if domain == "" {
		srv.httpError(w, "domain required", http.StatusBadRequest)
		return
	}

	ruleID := randomID()
	now := time.Now().Unix()

	srv.dbMu.Lock()
	defer srv.dbMu.Unlock()

	_, err := srv.db.ExecContext(r.Context(), `
		INSERT INTO rules (id, name, condition_type, condition_value, action, priority, enabled, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, ruleID, fmt.Sprintf("Quick block: %s", domain), "domain", domain, "block", 1, true, now)

	if err != nil {
		srv.Logf(LogLevelWarn, logPrefixHTTP, "failed to create block rule for %s: %v", domain, err)
		srv.httpError(w, "failed to create block rule", http.StatusInternalServerError)
		return
	}

	srv.Logf(LogLevelInfo, logPrefixHTTP, "query blocked: %s (rule: %s)", domain, ruleID)
	srv.httpJSON(w, map[string]string{"status": "ok"}, http.StatusOK)
}

func (srv *GuardianServer) handleTestDomain(w http.ResponseWriter, r *http.Request) {
	if !srv.verifyAuth(w, r) {
		return
	}

	domain := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("domain")))
	clientIP := strings.TrimSpace(r.URL.Query().Get("client_ip"))

	if domain == "" {
		srv.httpError(w, "domain parameter required", http.StatusBadRequest)
		return
	}

	var checks []string

	// Step 1: Check custom rules
	checks = append(checks, "1. Checking custom rules...")
	blockedByRule, allowedByRule, ruleReason := srv.checkCustomRules(domain, clientIP)
	if allowedByRule {
		checks = append(checks, "   ✓ Custom rule matched (allow exception)")
		checks = append(checks, "✓ Domain allowed by custom rule")
		srv.httpJSON(w, map[string]interface{}{
			"domain":     domain,
			"client_ip":  clientIP,
			"blocked":    false,
			"reason":     "",
			"category":   "",
			"confidence": 0,
			"checks":     checks,
		}, http.StatusOK)
		return
	} else if blockedByRule {
		checks = append(checks, "   ✓ Custom rule matched (block)")
		srv.httpJSON(w, map[string]interface{}{
			"domain":     domain,
			"client_ip":  clientIP,
			"blocked":    true,
			"reason":     ruleReason,
			"category":   "custom_rule",
			"confidence": 0,
			"checks":     checks,
		}, http.StatusOK)
		return
	} else {
		checks = append(checks, "   - No custom rules matched")
	}

	// Step 2: Check blocklist
	checks = append(checks, "2. Checking blocklist...")
	srv.blocklistMu.RLock()
	_, inBlocklist := srv.blocklist[domain]
	srv.blocklistMu.RUnlock()

	if inBlocklist {
		checks = append(checks, "   ✓ Domain matched in blocklist")
		srv.httpJSON(w, map[string]interface{}{
			"domain":     domain,
			"client_ip":  clientIP,
			"blocked":    true,
			"reason":     "blocklist",
			"category":   "blocklist",
			"confidence": 0,
			"checks":     checks,
		}, http.StatusOK)
		return
	}
	checks = append(checks, "   - Not in blocklist")

	// Step 3: Check ML classification
	checks = append(checks, "3. Running ML classification...")
	if srv.mlClient == nil {
		checks = append(checks, "   - ML service unavailable")
	} else {
		isMalicious, category, confidence, err := srv.classifyWithML(domain)
		if err != nil {
			checks = append(checks, fmt.Sprintf("   - ML classification failed: %v", err))
		} else if isMalicious {
			// Check if confidence meets threshold
			srv.mlThresholdMu.RLock()
			meetsThreshold := float64(confidence) >= srv.mlThreshold
			threshold := srv.mlThreshold
			srv.mlThresholdMu.RUnlock()

			if meetsThreshold {
				checks = append(checks, fmt.Sprintf("   ✓ ML detected as %s (%.0f%% confidence, threshold: %.0f%%)", category, confidence*100, threshold*100))
				srv.httpJSON(w, map[string]interface{}{
					"domain":     domain,
					"client_ip":  clientIP,
					"blocked":    true,
					"reason":     "ml:" + category,
					"category":   category,
					"confidence": confidence,
					"checks":     checks,
				}, http.StatusOK)
				return
			} else {
				checks = append(checks, fmt.Sprintf("   - ML detected as %s but below threshold (%.0f%% < %.0f%% threshold)", category, confidence*100, threshold*100))
			}
		} else {
			checks = append(checks, fmt.Sprintf("   - ML classified as safe (%.0f%% confidence)", confidence*100))
		}
	}

	checks = append(checks, "✓ Domain allowed")
	srv.httpJSON(w, map[string]interface{}{
		"domain":     domain,
		"client_ip":  clientIP,
		"blocked":    false,
		"reason":     "",
		"category":   "",
		"confidence": 0,
		"checks":     checks,
	}, http.StatusOK)
}

// ════════════════════════════════════════════════════════════════════════════════
// ─ HTTP handlers - Services management ────────────────────────────────────────
// ════════════════════════════════════════════════════════════════════════════════

func (srv *GuardianServer) registerServicesHandlers(mux *http.ServeMux, dev bool) {
	mux.HandleFunc("GET /api/services/definitions", srv.handleGetServiceDefinitions)
	mux.HandleFunc("GET /api/services", srv.handleGetServices)
	mux.HandleFunc("POST /api/services", srv.handleCreateService)
	mux.HandleFunc("DELETE /api/services", srv.handleDeleteService)
}

func (srv *GuardianServer) handleGetServiceDefinitions(w http.ResponseWriter, r *http.Request) {
	if !srv.verifyAuth(w, r) {
		return
	}

	srv.httpJSON(w, config.PredefinedServices, http.StatusOK)
}

func (srv *GuardianServer) handleGetServices(w http.ResponseWriter, r *http.Request) {
	if !srv.verifyAuth(w, r) {
		return
	}

	scope := r.URL.Query().Get("scope")
	if scope == "" {
		scope = "global"
	}
	scopeKey := r.URL.Query().Get("key")
	merged := r.URL.Query().Get("merged") == "1"

	srv.dbMu.RLock()
	defer srv.dbMu.RUnlock()

	// If merged=1, we return global AND scope-specific. Otherwise just the specific scope.
	var query string
	var args []interface{}

	if merged && scope != "global" {
		query = `
			SELECT service_id, enabled, day_of_week, start_hour, end_hour, scope
			FROM service_schedules
			WHERE scope = 'global' OR (scope = ? AND scope_key = ?)
		`
		args = []interface{}{scope, scopeKey}
	} else {
		query = `
			SELECT service_id, enabled, day_of_week, start_hour, end_hour, scope
			FROM service_schedules
			WHERE scope = ? AND scope_key = ?
		`
		args = []interface{}{scope, scopeKey}
	}

	rows, err := srv.db.QueryContext(srv.ctx, query, args...)
	if err != nil {
		srv.httpError(w, "database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	// For merged, scope-specific wins over global.
	schedules := make(map[string]map[string]interface{})
	for rows.Next() {
		var serviceID string
		var enabled bool
		var dayOfWeek, startHour, endHour sql.NullInt64
		var rowScope string

		if err := rows.Scan(&serviceID, &enabled, &dayOfWeek, &startHour, &endHour, &rowScope); err != nil {
			continue
		}

		// If it's already in the map and it's a global row, skip it (because specific scope wins).
		if existing, ok := schedules[serviceID]; ok {
			if rowScope == "global" && existing["source"] != "global" {
				continue
			}
		}

		schedules[serviceID] = map[string]interface{}{
			"enabled":      enabled,
			"days_of_week": "",
			"time_start":   "",
			"time_end":     "",
			"source":       rowScope,
		}
	}

	srv.httpJSON(w, schedules, http.StatusOK)
}

func (srv *GuardianServer) handleCreateService(w http.ResponseWriter, r *http.Request) {
	if !srv.verifyAuth(w, r) {
		return
	}

	var req struct {
		Scope      string `json:"scope"`
		ScopeKey   string `json:"scope_key"`
		ServiceID  string `json:"service_id"`
		Enabled    bool   `json:"enabled"`
		DaysOfWeek string `json:"days_of_week"`
		TimeStart  string `json:"time_start"`
		TimeEnd    string `json:"time_end"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		srv.httpError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Scope == "" {
		req.Scope = "global"
	}

	// Composite ID for conflict resolution
	id := req.Scope + ":" + req.ScopeKey + ":" + req.ServiceID

	srv.dbMu.Lock()
	defer srv.dbMu.Unlock()

	_, err := srv.db.ExecContext(srv.ctx, `
		INSERT INTO service_schedules (id, service_id, enabled, scope, scope_key)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET enabled=excluded.enabled, created_at=CURRENT_TIMESTAMP
	`, id, req.ServiceID, req.Enabled, req.Scope, req.ScopeKey)

	if err != nil {
		srv.Logf(LogLevelError, logPrefixHTTP, "failed to save service: %v", err)
		srv.httpError(w, "database error", http.StatusInternalServerError)
		return
	}

	srv.svcScheduleCacheMu.Lock()
	delete(srv.svcScheduleCache, req.ServiceID)
	srv.svcScheduleCacheMu.Unlock()

	srv.Logf(LogLevelInfo, logPrefixHTTP, "updated service schedule for %s", req.ServiceID)
	srv.httpStatus(w, http.StatusOK)
}

func (srv *GuardianServer) handleDeleteService(w http.ResponseWriter, r *http.Request) {
	if !srv.verifyAuth(w, r) {
		return
	}

	var req struct {
		Scope     string `json:"scope"`
		ScopeKey  string `json:"scope_key"`
		ServiceID string `json:"service_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.ServiceID = r.URL.Query().Get("id")
		if req.ServiceID == "" {
			req.ServiceID = r.URL.Query().Get("service_id")
		}
		req.Scope = r.URL.Query().Get("scope")
		req.ScopeKey = r.URL.Query().Get("scope_key")
	}

	if req.Scope == "" {
		req.Scope = "global"
	}

	if req.ServiceID == "" {
		srv.httpError(w, "missing service id", http.StatusBadRequest)
		return
	}

	id := req.Scope + ":" + req.ScopeKey + ":" + req.ServiceID

	srv.dbMu.Lock()
	defer srv.dbMu.Unlock()

	_, err := srv.db.ExecContext(srv.ctx, `
		DELETE FROM service_schedules WHERE id = ?
	`, id)

	if err != nil {
		srv.Logf(LogLevelError, logPrefixHTTP, "failed to delete service schedule: %v", err)
		srv.httpError(w, "database error", http.StatusInternalServerError)
		return
	}

	srv.svcScheduleCacheMu.Lock()
	delete(srv.svcScheduleCache, req.ServiceID)
	srv.svcScheduleCacheMu.Unlock()

	srv.Logf(LogLevelInfo, logPrefixHTTP, "deleted service schedule for %s", req.ServiceID)
	srv.httpStatus(w, http.StatusOK)
}

// ════════════════════════════════════════════════════════════════════════════════
// ─ HTTP handlers - Rules management ────────────────────────────────────────────
// ════════════════════════════════════════════════════════════════════════════════

func (srv *GuardianServer) registerRulesHandlers(mux *http.ServeMux, dev bool) {
	mux.HandleFunc("GET /api/rules", srv.handleGetRules)
	mux.HandleFunc("POST /api/rules", srv.handleCreateRule)
}

func (srv *GuardianServer) handleGetRules(w http.ResponseWriter, r *http.Request) {
	if !srv.verifyAuth(w, r) {
		return
	}

	scope := r.URL.Query().Get("scope")
	scopeKey := r.URL.Query().Get("key")

	key := "rules:global"
	if scope != "" && scope != "global" && scopeKey != "" {
		key = "rules:" + scope + ":" + scopeKey
	}

	srv.dbMu.RLock()
	defer srv.dbMu.RUnlock()

	var rules string
	err := srv.db.QueryRowContext(srv.ctx, "SELECT value FROM settings WHERE key = ?", key).Scan(&rules)
	if err != nil && err != sql.ErrNoRows {
		srv.httpError(w, "database error", http.StatusInternalServerError)
		return
	}

	srv.httpJSON(w, map[string]string{"rules": rules}, http.StatusOK)
}

func (srv *GuardianServer) handleCreateRule(w http.ResponseWriter, r *http.Request) {
	if !srv.verifyAuth(w, r) {
		return
	}

	var req struct {
		Scope    string `json:"scope_type"`
		ScopeKey string `json:"scope_key"`
		Rules    string `json:"rules"`
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		srv.httpError(w, "failed to read body", http.StatusBadRequest)
		return
	}

	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		srv.httpError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Fallback for settings page which just sends {"rules": "..."}
	if req.Scope == "" {
		req.Scope = "global"
	}

	key := "rules:global"
	if req.Scope != "global" && req.ScopeKey != "" {
		key = "rules:" + req.Scope + ":" + req.ScopeKey
	}

	srv.dbMu.Lock()
	defer srv.dbMu.Unlock()

	_, err = srv.db.ExecContext(srv.ctx, `
		INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=CURRENT_TIMESTAMP
	`, key, req.Rules)

	if err != nil {
		srv.httpError(w, "failed to save rules", http.StatusInternalServerError)
		return
	}

	srv.Logf(LogLevelInfo, logPrefixHTTP, "updated rules for %s", key)
	srv.httpStatus(w, http.StatusCreated)
}

// ════════════════════════════════════════════════════════════════════════════════
// ─ HTTP handlers - Client groups management ────────────────────────────────────
// ════════════════════════════════════════════════════════════════════════════════

func (srv *GuardianServer) registerGroupsHandlers(mux *http.ServeMux, dev bool) {
	mux.HandleFunc("GET /api/groups", srv.handleGetGroups)
	mux.HandleFunc("POST /api/groups", srv.handleCreateGroup)
	mux.HandleFunc("DELETE /api/groups", srv.handleDeleteGroup)
	mux.HandleFunc("POST /api/groups/members", srv.handleAddGroupMember)
	mux.HandleFunc("DELETE /api/groups/members", srv.handleRemoveGroupMember)
}

func (srv *GuardianServer) handleGetGroups(w http.ResponseWriter, r *http.Request) {
	if !srv.verifyAuth(w, r) {
		return
	}

	srv.dbMu.RLock()
	defer srv.dbMu.RUnlock()

	// Get all groups
	rows, err := srv.db.QueryContext(r.Context(), `
		SELECT id, name, description, created_at FROM client_groups ORDER BY created_at DESC
	`)
	if err != nil {
		srv.httpError(w, "failed to query groups", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	groups := []map[string]interface{}{}

	for rows.Next() {
		var id, name, description string
		var createdAt sql.NullInt64
		createdAtStr := ""

		if err := rows.Scan(&id, &name, &description, &createdAt); err != nil {
			continue
		}

		if createdAt.Valid {
			createdAtStr = time.Unix(createdAt.Int64, 0).Format(time.RFC3339)
		} else {
			createdAtStr = time.Now().Format(time.RFC3339)
		}

		// Get members (IPs) for this group
		memberRows, err := srv.db.QueryContext(r.Context(), `
			SELECT id, client_ip FROM client_group_members WHERE group_id = ? ORDER BY added_at ASC
		`, id)
		if err != nil {
			memberRows = nil
		}

		members := []map[string]interface{}{}
		if memberRows != nil {
			for memberRows.Next() {
				var memberID, clientIP string
				if err := memberRows.Scan(&memberID, &clientIP); err != nil {
					continue
				}
				members = append(members, map[string]interface{}{
					"id":         memberID,
					"identifier": clientIP,
					"type":       "ip",
				})
			}
			memberRows.Close()
		}

		groups = append(groups, map[string]interface{}{
			"id":          id,
			"name":        name,
			"description": description,
			"created_at":  createdAtStr,
			"members":     members,
		})
	}

	srv.httpJSON(w, groups, http.StatusOK)
}

func (srv *GuardianServer) handleCreateGroup(w http.ResponseWriter, r *http.Request) {
	if !srv.verifyAuth(w, r) {
		return
	}

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		srv.httpError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	id := randomID()
	now := time.Now().Unix()

	srv.dbMu.Lock()
	defer srv.dbMu.Unlock()

	// Create the group
	_, err := srv.db.ExecContext(r.Context(), `
		INSERT INTO client_groups (id, name, description, created_at)
		VALUES (?, ?, ?, ?)
	`, id, req.Name, req.Description, now)

	if err != nil {
		srv.httpError(w, "failed to create group", http.StatusInternalServerError)
		return
	}

	srv.Logf(LogLevelInfo, logPrefixHTTP, "created group %s (%s)", id, req.Name)
	srv.httpJSON(w, map[string]string{"id": id}, http.StatusCreated)
}

func (srv *GuardianServer) handleDeleteGroup(w http.ResponseWriter, r *http.Request) {
	if !srv.verifyAuth(w, r) {
		return
	}

	var req struct {
		ID interface{} `json:"id"`
	}

	groupID := r.URL.Query().Get("id")
	if groupID == "" {
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			switch v := req.ID.(type) {
			case string:
				groupID = v
			case float64:
				groupID = fmt.Sprintf("%.0f", v)
			}
		}
	}

	if groupID == "" {
		srv.httpError(w, "missing group id", http.StatusBadRequest)
		return
	}

	srv.dbMu.Lock()
	defer srv.dbMu.Unlock()

	_, err := srv.db.ExecContext(r.Context(), "DELETE FROM client_groups WHERE id = ?", groupID)
	if err != nil {
		srv.httpError(w, "failed to delete group", http.StatusInternalServerError)
		return
	}

	srv.Logf(LogLevelInfo, logPrefixHTTP, "deleted group %s", groupID)
	srv.httpStatus(w, http.StatusOK)
}

func (srv *GuardianServer) handleAddGroupMember(w http.ResponseWriter, r *http.Request) {
	if !srv.verifyAuth(w, r) {
		return
	}

	var req struct {
		GroupID    interface{} `json:"group_id"`
		Identifier string      `json:"identifier"`
		Type       string      `json:"type"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		srv.httpError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	var reqGroupID string
	switch v := req.GroupID.(type) {
	case string:
		reqGroupID = v
	case float64:
		reqGroupID = fmt.Sprintf("%.0f", v)
	}

	srv.dbMu.Lock()
	defer srv.dbMu.Unlock()

	// Verify the group exists
	var groupID string
	err := srv.db.QueryRowContext(r.Context(), `
		SELECT id FROM client_groups WHERE id = ? LIMIT 1
	`, reqGroupID).Scan(&groupID)
	if err != nil {
		srv.httpError(w, "group not found", http.StatusBadRequest)
		return
	}

	_, err = srv.db.ExecContext(r.Context(), `
		INSERT INTO client_group_members (id, group_id, client_ip, added_at)
		VALUES (?, ?, ?, ?)
	`, randomID(), reqGroupID, req.Identifier, time.Now().Unix())

	if err != nil {
		srv.httpError(w, "failed to add member", http.StatusInternalServerError)
		return
	}

	srv.Logf(LogLevelInfo, logPrefixHTTP, "added member %s to group %s", req.Identifier, reqGroupID)
	srv.httpJSON(w, map[string]string{"status": "ok"}, http.StatusOK)
}

func (srv *GuardianServer) handleRemoveGroupMember(w http.ResponseWriter, r *http.Request) {
	if !srv.verifyAuth(w, r) {
		return
	}

	var req struct {
		GroupID    interface{} `json:"group_id"`
		Identifier string      `json:"identifier"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		srv.httpError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	var groupID string
	switch v := req.GroupID.(type) {
	case string:
		groupID = v
	case float64:
		groupID = fmt.Sprintf("%.0f", v)
	}

	srv.dbMu.Lock()
	defer srv.dbMu.Unlock()

	_, err := srv.db.ExecContext(r.Context(), `
		DELETE FROM client_group_members WHERE group_id = ? AND client_ip = ?
	`, groupID, req.Identifier)

	if err != nil {
		srv.httpError(w, "failed to remove member", http.StatusInternalServerError)
		return
	}

	srv.Logf(LogLevelInfo, logPrefixHTTP, "removed member %s from group %s", req.Identifier, groupID)
	srv.httpJSON(w, map[string]string{"status": "ok"}, http.StatusOK)
}

// ════════════════════════════════════════════════════════════════════════════════
// ─ HTTP handlers - Retention management ────────────────────────────────────────
// ════════════════════════════════════════════════════════════════════════════════

func (srv *GuardianServer) registerRetentionHandlers(mux *http.ServeMux, dev bool) {
	mux.HandleFunc("GET /api/retention", srv.handleGetRetention)
	mux.HandleFunc("POST /api/retention", srv.handleSetRetention)
}

func (srv *GuardianServer) handleGetRetention(w http.ResponseWriter, r *http.Request) {
	if !srv.verifyAuth(w, r) {
		return
	}

	srv.httpJSON(w, map[string]interface{}{
		"days":    30,
		"enabled": true,
	}, http.StatusOK)
}

func (srv *GuardianServer) handleSetRetention(w http.ResponseWriter, r *http.Request) {
	if !srv.verifyAuth(w, r) {
		return
	}

	var req struct {
		Days    int  `json:"days"`
		Enabled bool `json:"enabled"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		srv.httpError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	srv.Logf(LogLevelInfo, logPrefixHTTP, "retention settings updated: %d days", req.Days)
	srv.httpJSON(w, map[string]string{"status": "ok"}, http.StatusOK)
}

// ════════════════════════════════════════════════════════════════════════════════
// ─ HTTP handlers - Frontend and system ────────────────────────────────────────
// ════════════════════════════════════════════════════════════════════════════════

func (srv *GuardianServer) serveAssets(fsys fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Remove leading slash if present
		filePath := strings.TrimPrefix(r.URL.Path, "/")
		// Add "assets/" prefix since we're in the frontend/dist subdirectory
		filePath = "assets/" + filePath
		srv.Logf(LogLevelDebug, logPrefixHTTP, "serving asset: %s", filePath)

		// Determine MIME type based on file extension
		var contentType string
		if strings.HasSuffix(filePath, ".js") {
			contentType = "application/javascript; charset=utf-8"
		} else if strings.HasSuffix(filePath, ".css") {
			contentType = "text/css; charset=utf-8"
		} else if strings.HasSuffix(filePath, ".svg") {
			contentType = "image/svg+xml"
		} else if strings.HasSuffix(filePath, ".json") {
			contentType = "application/json; charset=utf-8"
		} else if strings.HasSuffix(filePath, ".png") {
			contentType = "image/png"
		} else if strings.HasSuffix(filePath, ".jpg") || strings.HasSuffix(filePath, ".jpeg") {
			contentType = "image/jpeg"
		} else if strings.HasSuffix(filePath, ".woff") {
			contentType = "font/woff"
		} else if strings.HasSuffix(filePath, ".woff2") {
			contentType = "font/woff2"
		} else if strings.HasSuffix(filePath, ".ttf") {
			contentType = "font/ttf"
		} else if strings.HasSuffix(filePath, ".eot") {
			contentType = "application/vnd.ms-fontobject"
		} else {
			contentType = "text/plain; charset=utf-8"
		}

		srv.Logf(LogLevelDebug, logPrefixHTTP, "detected content-type: %s for %s", contentType, filePath)

		// Read file from embedded filesystem
		data, err := fs.ReadFile(fsys, filePath)
		if err != nil {
			srv.Logf(LogLevelWarn, logPrefixHTTP, "asset not found: %s (%v)", filePath, err)
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintf(w, "not found")
			return
		}

		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "public, max-age=3600")
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	})
}

func (srv *GuardianServer) serveStaticFile(fsys fs.FS) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Remove leading slash if present
		filePath := strings.TrimPrefix(r.URL.Path, "/")
		srv.Logf(LogLevelDebug, logPrefixHTTP, "serving static file: %s", filePath)

		// Determine MIME type based on file extension
		var contentType string
		if strings.HasSuffix(filePath, ".svg") {
			contentType = "image/svg+xml"
		} else if strings.HasSuffix(filePath, ".ico") {
			contentType = "image/x-icon"
		} else if strings.HasSuffix(filePath, ".png") {
			contentType = "image/png"
		} else if strings.HasSuffix(filePath, ".jpg") || strings.HasSuffix(filePath, ".jpeg") {
			contentType = "image/jpeg"
		} else if strings.HasSuffix(filePath, ".woff") {
			contentType = "font/woff"
		} else if strings.HasSuffix(filePath, ".woff2") {
			contentType = "font/woff2"
		} else {
			contentType = "application/octet-stream"
		}

		// Read file from embedded filesystem
		data, err := fs.ReadFile(fsys, filePath)
		if err != nil {
			srv.Logf(LogLevelWarn, logPrefixHTTP, "static file not found: %s (%v)", filePath, err)
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintf(w, "not found")
			return
		}

		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "public, max-age=31536000")
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	}
}

func (srv *GuardianServer) registerFrontendHandlers(mux *http.ServeMux, dev bool) {
	if dev {
		mux.HandleFunc("/", srv.handleDevRedirect)
		mux.HandleFunc("/index.html", srv.handleDevRedirect)
		mux.HandleFunc("/assets/", srv.handleDevRedirect)
		mux.HandleFunc("/favicon.svg", srv.handleDevRedirect)
	} else {
		mux.HandleFunc("/", srv.handleFrontend)
		mux.HandleFunc("/index.html", srv.handleFrontend)

		fsys, _ := fs.Sub(srv.embeddedDist, "frontend/dist")
		mux.Handle("/assets/", http.StripPrefix("/assets/", srv.serveAssets(fsys)))
		mux.HandleFunc("/favicon.svg", srv.serveStaticFile(fsys))
	}
}

func (srv *GuardianServer) handleFrontend(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/index.html" && !strings.HasPrefix(r.URL.Path, "/api") && !strings.HasPrefix(r.URL.Path, "/assets") {
		r.URL.Path = "/"
	}

	file, err := srv.embeddedDist.ReadFile("frontend/dist/index.html")
	if err != nil {
		srv.httpError(w, "frontend not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write(file)
}

func (srv *GuardianServer) handleDevRedirect(w http.ResponseWriter, r *http.Request) {
	devURL := "http://localhost:5173" + r.URL.Path
	http.Redirect(w, r, devURL, http.StatusTemporaryRedirect)
}

// ════════════════════════════════════════════════════════════════════════════════
// ─ HTTP utility functions ────────────────────────────────────────────────────
// ════════════════════════════════════════════════════════════════════════════════

func (srv *GuardianServer) verifyAuth(w http.ResponseWriter, r *http.Request) bool {
	sessionCookie, err := r.Cookie(config.SessionCookieName)
	if err != nil {
		srv.httpError(w, "unauthorized", http.StatusUnauthorized)
		return false
	}

	srv.sessionMu.RLock()
	session, exists := srv.sessions[sessionCookie.Value]
	srv.sessionMu.RUnlock()

	if !exists || session.ExpiresAt.Before(time.Now()) {
		srv.httpError(w, "session expired", http.StatusUnauthorized)
		return false
	}

	return true
}

func (srv *GuardianServer) httpError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func (srv *GuardianServer) httpJSON(w http.ResponseWriter, data interface{}, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(data)
}

func (srv *GuardianServer) httpStatus(w http.ResponseWriter, statusCode int) {
	w.WriteHeader(statusCode)
}

// ════════════════════════════════════════════════════════════════════════════════
// ─ Query logging and statistics ───────────────────────────────────────────────
// ════════════════════════════════════════════════════════════════════════════════

func (srv *GuardianServer) recordQuery(
	domain string,
	clientIP string,
	queryType uint16,
	result string,
	blockedBy string,
	isMalicious bool,
	mlCategory string,
	mlConfidence float32,
) {
	go func() {
		id := randomID()
		queryTypeStr := dns.TypeToString[queryType]

		srv.dbMu.Lock()
		defer srv.dbMu.Unlock()

		_, err := srv.db.ExecContext(srv.ctx, `
			INSERT INTO queries (id, domain, client_ip, query_type, result, blocked_by, is_malicious, ml_category, ml_confidence, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, id, domain, clientIP, queryTypeStr, result, blockedBy, isMalicious, mlCategory, mlConfidence, time.Now().Unix())

		if err != nil {
			srv.Logf(LogLevelWarn, logPrefixDB, "failed to record query: %v", err)
		}
	}()
}

func (srv *GuardianServer) recordStats(fn func(*ServerStats)) {
	srv.statsMu.Lock()
	defer srv.statsMu.Unlock()
	fn(&srv.stats)
	srv.stats.LastUpdateTime = time.Now()
}

// ════════════════════════════════════════════════════════════════════════════════
// ─ Server startup ────────────────────────────────────────────────────────────
// ════════════════════════════════════════════════════════════════════════════════

func (srv *GuardianServer) StartServers(listen, webAddr string, devCORS, setupDefaultUser bool) error {
	mux := http.NewServeMux()

	var handler http.Handler = mux
	if devCORS {
		handler = corsMiddleware(mux)
	}

	srv.registerBlocklistHandlers(mux, devCORS)
	srv.registerUpstreamHandlers(mux, devCORS)
	srv.registerMLHandlers(mux, devCORS)
	srv.registerAuthHandlers(mux, devCORS)
	srv.registerQueryHandlers(mux, devCORS)
	srv.registerServicesHandlers(mux, devCORS)
	srv.registerRulesHandlers(mux, devCORS)
	srv.registerGroupsHandlers(mux, devCORS)
	srv.registerRetentionHandlers(mux, devCORS)
	srv.registerFrontendHandlers(mux, devCORS)

	go func() {
		srv.Logf(LogLevelInfo, logPrefixHTTP, "starting HTTP server on %s", webAddr)
		httpServer := &http.Server{
			Addr:         webAddr,
			Handler:      handler,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 15 * time.Second,
			IdleTimeout:  60 * time.Second,
		}

		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			srv.Logf(LogLevelError, logPrefixHTTP, "HTTP server error: %v", err)
		}
	}()

	time.Sleep(ServerStartupWait)

	if err := srv.startDNSServers(listen); err != nil {
		return err
	}

	return nil
}

func (srv *GuardianServer) startDNSServers(listen string) error {
	dnsServer := &dns.Server{
		Addr:         listen,
		Net:          "udp",
		Handler:      dns.HandlerFunc(srv.handleDNSQuery),
		ReadTimeout:  DNSTimeout,
		WriteTimeout: DNSTimeout,
	}

	go func() {
		srv.Logf(LogLevelInfo, logPrefixDNS, "starting DNS server on %s (UDP)", listen)
		if err := dnsServer.ListenAndServe(); err != nil {
			srv.Logf(LogLevelError, logPrefixDNS, "DNS server error: %v", err)
		}
	}()

	tcpServer := &dns.Server{
		Addr:         listen,
		Net:          "tcp",
		Handler:      dns.HandlerFunc(srv.handleDNSQuery),
		ReadTimeout:  DNSTimeout,
		WriteTimeout: DNSTimeout,
	}

	go func() {
		srv.Logf(LogLevelInfo, logPrefixDNS, "starting DNS server on %s (TCP)", listen)
		if err := tcpServer.ListenAndServe(); err != nil {
			srv.Logf(LogLevelError, logPrefixDNS, "DNS server error: %v", err)
		}
	}()

	time.Sleep(ServerStartupWait)
	return nil
}

// ════════════════════════════════════════════════════════════════════════════════
// ─ Logging utility ───────────────────────────────────────────────────────────
// ════════════════════════════════════════════════════════════════════════════════

func (srv *GuardianServer) Logf(level LogLevel, prefix string, format string, args ...interface{}) {
	if level > srv.logLevel {
		return
	}

	levelName := LogLevelNames[level]
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	message := fmt.Sprintf(format, args...)
	log.Printf("[%s] %s %s %s", timestamp, levelName, prefix, message)
}

// ════════════════════════════════════════════════════════════════════════════════
// ─ Helper functions for common utilities ─────────────────────────────────────
// ════════════════════════════════════════════════════════════════════════════════

func parseSessionTimestamp(val string) (time.Time, error) {
	// Try parsing as RFC3339 first (new format)
	if t, err := time.Parse(time.RFC3339, val); err == nil {
		return t, nil
	}

	// Try parsing as Unix timestamp (old format)
	if unixTime, err := strconv.ParseInt(val, 10, 64); err == nil {
		return time.Unix(unixTime, 0), nil
	}

	return time.Time{}, fmt.Errorf("unable to parse timestamp: %s", val)
}

func randomID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func hashPassword(password string) string {
	return base64.StdEncoding.EncodeToString([]byte(password))
}

func verifyPassword(hash, password string) bool {
	expected, _ := base64.StdEncoding.DecodeString(hash)
	return string(expected) == password
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "http://localhost:5173")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Credentials", "true")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// ════════════════════════════════════════════════════════════════════════════════
// ─ Embedded Python and ML service helpers ────────────────────────────────────
// ════════════════════════════════════════════════════════════════════════════════

func StartEmbeddedPython(tempDir string) (*exec.Cmd, error) {
	pythonExe := "python3"
	if runtime.GOOS == "windows" {
		pythonExe = "python"
	}

	if _, err := exec.LookPath(pythonExe); err != nil {
		return nil, fmt.Errorf("python not found: %w", err)
	}

	mainPy := filepath.Join(tempDir, "guardian_service.py")
	cmd := exec.Command(pythonExe, mainPy)
	cmd.Dir = tempDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	log.Printf("[ml] started embedded Python service (PID: %d)", cmd.Process.Pid)
	return cmd, nil
}

func WriteEmbeddedDir(embeddedFS embed.FS, srcDir, destDir string) error {
	return fs.WalkDir(embeddedFS, srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		relPath, _ := filepath.Rel(srcDir, path)
		destPath := filepath.Join(destDir, relPath)

		if d.IsDir() {
			return os.MkdirAll(destPath, 0o755)
		}

		data, err := embeddedFS.ReadFile(path)
		if err != nil {
			return err
		}

		return os.WriteFile(destPath, data, 0o644)
	})
}

func IsTermux() bool {
	_, err := os.Stat("/data/data/com.termux/files")
	return err == nil
}

// ════════════════════════════════════════════════════════════════════════════════
// ─ Service schedule management ───────────────────────────────────────────────
// ════════════════════════════════════════════════════════════════════════════════

type ServiceSchedule struct {
	ID        string
	ServiceID string
	Scope     string
	ScopeKey  string
	DayOfWeek int
	StartHour int
	EndHour   int
	Enabled   bool
}

func (srv *GuardianServer) getServiceSchedules(serviceID string) ([]ServiceSchedule, error) {
	srv.svcScheduleCacheMu.Lock()
	if cached, ok := srv.svcScheduleCache[serviceID]; ok && time.Since(srv.lastSvcScheduleFetch) < config.SvcScheduleCacheTTL {
		defer srv.svcScheduleCacheMu.Unlock()
		return []ServiceSchedule{cached}, nil
	}
	srv.svcScheduleCacheMu.Unlock()

	srv.dbMu.RLock()
	defer srv.dbMu.RUnlock()

	rows, err := srv.db.QueryContext(srv.ctx, `
		SELECT id, service_id, scope, scope_key, day_of_week, start_hour, end_hour, enabled
		FROM service_schedules WHERE service_id = ?
	`, serviceID)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	schedules := []ServiceSchedule{}
	for rows.Next() {
		var s ServiceSchedule
		var dayOfWeek, startHour, endHour sql.NullInt64
		var scope, scopeKey sql.NullString
		if err := rows.Scan(&s.ID, &s.ServiceID, &scope, &scopeKey, &dayOfWeek, &startHour, &endHour, &s.Enabled); err != nil {
			continue
		}
		if scope.Valid {
			s.Scope = scope.String
		}
		if scopeKey.Valid {
			s.ScopeKey = scopeKey.String
		}
		if dayOfWeek.Valid {
			s.DayOfWeek = int(dayOfWeek.Int64)
		}
		if startHour.Valid {
			s.StartHour = int(startHour.Int64)
		}
		if endHour.Valid {
			s.EndHour = int(endHour.Int64)
		}
		schedules = append(schedules, s)
	}

	return schedules, nil
}

// ════════════════════════════════════════════════════════════════════════════════
// ─ Client group management ───────────────────────────────────────────────────
// ════════════════════════════════════════════════════════════════════════════════

type ClientGroup struct {
	ID          string
	Name        string
	Description string
	CreatedAt   time.Time
}

func (srv *GuardianServer) getClientGroup(groupID string) (*ClientGroup, error) {
	srv.dbMu.RLock()
	defer srv.dbMu.RUnlock()

	var g ClientGroup
	var createdAt int64
	err := srv.db.QueryRowContext(srv.ctx, `
		SELECT id, name, description, created_at FROM client_groups WHERE id = ? LIMIT 1
	`, groupID).Scan(&g.ID, &g.Name, &g.Description, &createdAt)

	if err != nil {
		return nil, err
	}

	g.CreatedAt = time.Unix(createdAt, 0)
	return &g, nil
}

func (srv *GuardianServer) getClientGroupMembers(groupID string) ([]string, error) {
	srv.dbMu.RLock()
	defer srv.dbMu.RUnlock()

	rows, err := srv.db.QueryContext(srv.ctx, `
		SELECT client_ip FROM client_group_members WHERE group_id = ? ORDER BY added_at
	`, groupID)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ips := []string{}
	for rows.Next() {
		var ip string
		if err := rows.Scan(&ip); err != nil {
			continue
		}
		ips = append(ips, ip)
	}

	return ips, nil
}

// ════════════════════════════════════════════════════════════════════════════════
// ─ Settings management ───────────────────────────────────────────────────────
// ════════════════════════════════════════════════════════════════════════════════

func (srv *GuardianServer) getSetting(key string) (string, error) {
	srv.dbMu.RLock()
	defer srv.dbMu.RUnlock()

	var value string
	err := srv.db.QueryRowContext(srv.ctx, "SELECT value FROM settings WHERE key = ?", key).Scan(&value)
	if err != nil && err != sql.ErrNoRows {
		return "", err
	}

	return value, nil
}

func (srv *GuardianServer) setSetting(key, value string) error {
	srv.dbMu.Lock()
	defer srv.dbMu.Unlock()

	_, err := srv.db.ExecContext(srv.ctx, `
		INSERT OR REPLACE INTO settings (key, value, updated_at)
		VALUES (?, ?, ?)
	`, key, value, time.Now().Format(time.RFC3339))

	return err
}

// ════════════════════════════════════════════════════════════════════════════════
// ─ Rules management ──────────────────────────────────────────────────────────
// ════════════════════════════════════════════════════════════════════════════════

type Rule struct {
	ID             string
	Name           string
	ConditionType  string
	ConditionValue string
	Action         string
	Priority       int
	Enabled        bool
	CreatedAt      time.Time
}

func (srv *GuardianServer) getEnabledRules() ([]Rule, error) {
	srv.dbMu.RLock()
	defer srv.dbMu.RUnlock()

	rows, err := srv.db.QueryContext(srv.ctx, `
		SELECT id, name, condition_type, condition_value, action, priority, enabled, created_at
		FROM rules WHERE enabled = 1 ORDER BY priority DESC
	`)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rules := []Rule{}
	for rows.Next() {
		var r Rule
		if err := rows.Scan(&r.ID, &r.Name, &r.ConditionType, &r.ConditionValue, &r.Action, &r.Priority, &r.Enabled, &r.CreatedAt); err != nil {
			continue
		}
		rules = append(rules, r)
	}

	return rules, nil
}

// ════════════════════════════════════════════════════════════════════════════════
// ─ Shutdown and cleanup ──────────────────────────────────────────────────────
// ════════════════════════════════════════════════════════════════════════════════

func (srv *GuardianServer) Close() error {
	srv.cancel()

	if srv.mlConn != nil {
		srv.mlConn.Close()
	}

	if srv.db != nil {
		srv.db.Close()
	}

	return nil
}
