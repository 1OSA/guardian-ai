package main

import (
	"bytes"
	"container/list"
	"context"
	"crypto/rand"
	"database/sql"
	"embed"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	iofs "io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	_ "modernc.org/sqlite"

	"github.com/miekg/dns"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	guardian "github.com/1OSA/guardian-ai/proto"
)

// AppVersion is set at build time via -ldflags "-X main.AppVersion=x.y.z".
// Falls back to "dev" when not injected.
var AppVersion = "dev"

//go:embed frontend/dist**
var embeddedDist embed.FS

//go:embed ml-service/guardian_grpc.py ml-service/guardian_pb2.py ml-service/guardian_pb2_grpc.py ml-service/requirements.txt ml-service/guardian_model.onnx ml-service/tokenizer.pickle
var embeddedML embed.FS

// ── Constants ─────────────────────────────────────────────────────────────────

const (
	defaultDBPath       = "guardian.db"
	defaultBlockfile    = "blocklists/hosts.txt"
	sessionCookieName   = "guardian_session"
	sessionTTL          = 24 * time.Hour
	mlCacheTTL          = 5 * time.Minute
	maxMLCacheSize      = 10_000
	dnsCacheTTL         = 60 * time.Second
	maxDNSCacheSize     = 50_000
	rateLimitPerMin     = 1000
	rateLimitBurst      = 200 // token-bucket burst capacity per client
	svcScheduleCacheTTL = 10 * time.Second
)

// ── Predefined blocklist sources ──────────────────────────────────────────────

// predefinedBlocklists is the static list of well-known blocklist sources
// offered in the Settings UI. Defined at package level so it is not
// re-allocated on every HTTP request.
var predefinedBlocklists = []map[string]string{
	{"name": "AdGuard DNS filter", "url": "https://adguardteam.github.io/HostlistsRegistry/assets/filter_1.txt"},
	{"name": "Steven Black's List", "url": "https://adguardteam.github.io/HostlistsRegistry/assets/filter_33.txt"},
	{"name": "OISD Blocklist Small", "url": "https://adguardteam.github.io/HostlistsRegistry/assets/filter_5.txt"},
	{"name": "Phishing URL Blocklist (PhishTank/OpenPhish)", "url": "https://adguardteam.github.io/HostlistsRegistry/assets/filter_30.txt"},
	{"name": "Dandelion Sprout's Anti-Malware List", "url": "https://adguardteam.github.io/HostlistsRegistry/assets/filter_12.txt"},
	{"name": "Phishing Army", "url": "https://adguardteam.github.io/HostlistsRegistry/assets/filter_18.txt"},
	{"name": "Malicious URL Blocklist (URLHaus)", "url": "https://adguardteam.github.io/HostlistsRegistry/assets/filter_11.txt"},
	{"name": "NoCoin Filter List", "url": "https://adguardteam.github.io/HostlistsRegistry/assets/filter_8.txt"},
	{"name": "HaGeZi's Normal Blocklist", "url": "https://adguardteam.github.io/HostlistsRegistry/assets/filter_34.txt"},
}

// ── Predefined services ───────────────────────────────────────────────────────

// PredefinedService describes a named service and the domains it uses.
type PredefinedService struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Icon     string   `json:"icon"`
	Category string   `json:"category"`
	Domains  []string `json:"domains"`
}

// PredefinedServices is the master list of blockable services.
var PredefinedServices = []PredefinedService{
	{
		ID: "roblox", Name: "Roblox", Icon: "🎮", Category: "Gaming",
		Domains: []string{
			"roblox.com", "roblox.com.br", "rbxcdn.com", "robloxlabs.com",
			"rbx.com", "robloxapp.com", "roblox.app",
		},
	},
	{
		ID: "netflix", Name: "Netflix", Icon: "🎬", Category: "Streaming",
		Domains: []string{
			"netflix.com", "nflxvideo.net", "nflximg.net", "nflximg.com",
			"nflxso.net", "nflxext.com", "netflixdns.net",
		},
	},
	{
		ID: "youtube", Name: "YouTube", Icon: "▶️", Category: "Streaming",
		Domains: []string{
			"youtube.com", "youtu.be", "ytimg.com", "googlevideo.com",
			"youtube-nocookie.com", "youtube.googleapis.com",
		},
	},
	{
		ID: "tiktok", Name: "TikTok", Icon: "🎵", Category: "Social",
		Domains: []string{
			"tiktok.com", "tiktokcdn.com", "tiktokv.com", "musical.ly",
			"muscdn.com", "byteoversea.com", "ibytedtos.com", "ibyteimg.com",
		},
	},
	{
		ID: "instagram", Name: "Instagram", Icon: "📸", Category: "Social",
		Domains: []string{
			"instagram.com", "cdninstagram.com", "igsonar.com",
		},
	},
	{
		ID: "facebook", Name: "Facebook", Icon: "👤", Category: "Social",
		Domains: []string{
			"facebook.com", "fbcdn.net", "fbsbx.com", "facebook.net",
			"facebookcorewwwi.onion", "fb.com", "fb.me",
		},
	},
	{
		ID: "twitter", Name: "X / Twitter", Icon: "🐦", Category: "Social",
		Domains: []string{
			"twitter.com", "x.com", "t.co", "twimg.com", "abs.twimg.com",
		},
	},
	{
		ID: "discord", Name: "Discord", Icon: "💬", Category: "Gaming",
		Domains: []string{
			"discord.com", "discordapp.com", "discordapp.net",
			"discord.gg", "discordstatus.com",
		},
	},
	{
		ID: "steam", Name: "Steam", Icon: "🕹️", Category: "Gaming",
		Domains: []string{
			"steampowered.com", "steamcommunity.com", "steamstatic.com",
			"steamusercontent.com", "steam-chat.com", "valvesoftware.com",
		},
	},
	{
		ID: "twitch", Name: "Twitch", Icon: "📡", Category: "Streaming",
		Domains: []string{
			"twitch.tv", "twitchapps.com", "jtvnw.net", "twitchsvc.net",
			"twitchstatus.com",
		},
	},
	{
		ID: "snapchat", Name: "Snapchat", Icon: "👻", Category: "Social",
		Domains: []string{
			"snapchat.com", "snap.com", "snapkit.com",
		},
	},
	{
		ID: "whatsapp", Name: "WhatsApp", Icon: "📱", Category: "Social",
		Domains: []string{
			"whatsapp.com", "whatsapp.net",
		},
	},
	{
		ID: "spotify", Name: "Spotify", Icon: "🎧", Category: "Streaming",
		Domains: []string{
			"spotify.com", "spotifycdn.com", "scdn.co", "byspotify.com",
		},
	},
	{
		ID: "minecraft", Name: "Minecraft", Icon: "⛏️", Category: "Gaming",
		Domains: []string{
			"minecraft.net", "minecraftservices.com", "mojang.com",
		},
	},
	{
		ID: "fortnite", Name: "Fortnite / Epic", Icon: "🏆", Category: "Gaming",
		Domains: []string{
			"epicgames.com", "fortnite.com", "fn.epicgames.com",
			"unrealengine.com", "helpshift.com",
		},
	},
	{
		ID: "reddit", Name: "Reddit", Icon: "🤖", Category: "Social",
		Domains: []string{
			"reddit.com", "redd.it", "redditmedia.com", "reddituploads.com",
			"redditstatic.com",
		},
	},
	{
		ID: "amazon_prime", Name: "Amazon Prime Video", Icon: "📦", Category: "Streaming",
		Domains: []string{
			"primevideo.com", "aiv-cdn.net", "amazon.com",
			"media-amazon.com", "amazonvideo.com",
		},
	},
	{
		ID: "disneyplus", Name: "Disney+", Icon: "✨", Category: "Streaming",
		Domains: []string{
			"disneyplus.com", "bamgrid.com", "dssott.com",
		},
	},
}

// ── ML allowlist ──────────────────────────────────────────────────────────────

// serviceDomainIndex maps domain -> []service_id for fast lookup in the DNS path.
var serviceDomainIndex map[string][]string

// mlAllowlist holds domains that should never be sent to the ML model.
// These are well-known legitimate domains; the model produces false positives on
// many of them (e.g. roblox.com scores as "Phishing" due to training-data bias).
var mlAllowlist map[string]struct{}

// extraMLAllowlist is the static set of platform/CDN/infrastructure domains
// added to mlAllowlist at init time in addition to all predefined service domains.
var extraMLAllowlist = []string{
	// Google
	"google.com", "googleapis.com", "googleusercontent.com", "gstatic.com",
	"googlevideo.com", "googletagmanager.com", "googletagservices.com",
	"googleadservices.com", "googlesyndication.com", "google-analytics.com",
	"doubleclick.net", "ggpht.com", "goo.gl", "g.co", "gmail.com",
	"googlemail.com", "google.co.uk", "google.ca", "google.com.au",
	// Microsoft / Windows / Xbox
	"microsoft.com", "microsoftonline.com", "microsoftedge.com",
	"windows.com", "windowsupdate.com", "windowsazure.com",
	"azure.com", "azureedge.net", "msftconnecttest.com",
	"msftncsi.com", "live.com", "outlook.com", "hotmail.com",
	"xbox.com", "xboxlive.com", "xboxservices.com",
	"office.com", "office365.com", "sharepoint.com", "onedrive.com",
	"skype.com", "teams.microsoft.com", "bing.com", "msn.com",
	// Apple
	"apple.com", "icloud.com", "mzstatic.com", "aaplimg.com",
	"apple-dns.net", "applecdn.net",
	// Cloudflare
	"cloudflare.com", "cloudflare-dns.com", "cloudflarestatus.com",
	"cloudflareinsights.com", "cfdata.org",
	// Akamai / CDN
	"akamai.com", "akamaized.net", "akamaihd.net", "akamaistream.net",
	"edgekey.net", "edgesuite.net",
	// Amazon AWS
	"amazonaws.com", "aws.amazon.com", "awsstatic.com", "cloudfront.net",
	// Fastly CDN
	"fastly.net", "fastlylb.net",
	// DNS / Infrastructure
	"1.1.1.1", "8.8.8.8", "9.9.9.9",
	"quad9.net", "opendns.com", "neustar.biz",
	// Browser safe-browsing / update endpoints
	"safebrowsing.googleapis.com", "safebrowsing.google.com",
	"update.googleapis.com", "clients1.google.com", "clients2.google.com",
	"clients3.google.com", "clients4.google.com",
	// Social / comms (beyond predefined services)
	"linkedin.com", "licdn.com",
	"pinterest.com", "pinimg.com",
	"tumblr.com",
	"telegram.org", "t.me",
	"signal.org",
	"zoom.us", "zoom.com", "zoomgov.com",
	"slack.com", "slack-edge.com", "slack-msgs.com",
	// Gaming (beyond predefined services)
	"ea.com", "origin.com", "eaassets-a.akamaihd.net",
	"playstation.com", "playstation.net", "sonyentertainmentnetwork.com",
	"nintendo.com", "nintendo.net",
	"blizzard.com", "battle.net", "bnet.com",
	"riotgames.com", "leagueoflegends.com", "valorant.com",
	// Payment / banking (common false-positive targets)
	"paypal.com", "paypalobjects.com",
	"stripe.com", "stripecdn.com",
	// Software delivery
	"github.com", "githubusercontent.com", "githubassets.com",
	"gitlab.com",
	"npmjs.com", "npmjs.org",
	"pypi.org", "pythonhosted.org",
	"golang.org", "go.dev",
	// Analytics / tag managers (often unusual-looking domains)
	"segment.com", "segment.io",
	"mixpanel.com",
	"amplitude.com",
	"intercom.io", "intercomcdn.com",
	"hotjar.com",
	// Ad/consent (commonly mis-flagged but not malicious)
	"cookielaw.org", "onetrust.com",
	// General TLD infrastructure
	"verisign.com", "icann.org",
}

// mlAllowlistSuffixes is a sorted slice of ".domain" strings built from
// mlAllowlist at init time, used for O(log n) subdomain matching.
var mlAllowlistSuffixes []string

func init() {
	// Build service domain index and seed ML allowlist from all predefined service domains.
	serviceDomainIndex = make(map[string][]string)
	mlAllowlist = make(map[string]struct{})

	for _, svc := range PredefinedServices {
		for _, d := range svc.Domains {
			d = strings.ToLower(d)
			serviceDomainIndex[d] = append(serviceDomainIndex[d], svc.ID)
			mlAllowlist[d] = struct{}{}
		}
	}

	for _, d := range extraMLAllowlist {
		mlAllowlist[strings.ToLower(d)] = struct{}{}
	}

	// Build sorted suffix slice for binary-search subdomain lookup.
	mlAllowlistSuffixes = make([]string, 0, len(mlAllowlist))
	for d := range mlAllowlist {
		mlAllowlistSuffixes = append(mlAllowlistSuffixes, "."+d)
	}
	sort.Strings(mlAllowlistSuffixes)
}

// isMLAllowlisted returns true if domain exactly matches or is a subdomain of
// any entry in mlAllowlist. Uses binary search for O(log n) performance.
func isMLAllowlisted(domain string) bool {
	domain = strings.ToLower(strings.TrimSuffix(domain, "."))
	if _, ok := mlAllowlist[domain]; ok {
		return true
	}
	for i := 0; i < len(domain); i++ {
		if domain[i] == '.' {
			suffix := domain[i:] // e.g. ".roblox.com"
			idx := sort.SearchStrings(mlAllowlistSuffixes, suffix)
			if idx < len(mlAllowlistSuffixes) && mlAllowlistSuffixes[idx] == suffix {
				return true
			}
		}
	}
	return false
}

// ── Log level ─────────────────────────────────────────────────────────────────

type LogLevel int

const (
	LogLevelError LogLevel = iota
	LogLevelWarn
	LogLevelInfo
	LogLevelDebug
)

var logLevelNames = [...]string{"ERROR", "WARN", "INFO", "DEBUG"}

// ── LRU cache ─────────────────────────────────────────────────────────────────

type dnsCacheEntry struct {
	msg       *dns.Msg
	expiresAt time.Time
}

// lruCache is a generic O(1) LRU cache backed by a doubly-linked list + map.
// The zero value is not usable; use newLRUCache.
type lruCache[K comparable, V any] struct {
	cap   int
	mu    sync.Mutex
	items map[K]*list.Element
	order *list.List // front = LRU (oldest), back = MRU (newest)
}

type lruEntry[K comparable, V any] struct {
	key   K
	value V
}

func newLRUCache[K comparable, V any](capacity int) *lruCache[K, V] {
	return &lruCache[K, V]{
		cap:   capacity,
		items: make(map[K]*list.Element, capacity),
		order: list.New(),
	}
}

// get retrieves a value and promotes it to MRU. Returns zero-value + false on miss.
func (c *lruCache[K, V]) get(key K) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		var zero V
		return zero, false
	}
	c.order.MoveToBack(el)
	return el.Value.(*lruEntry[K, V]).value, true
}

// set inserts or updates a value, evicting the LRU entry when over capacity.
func (c *lruCache[K, V]) set(key K, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		el.Value.(*lruEntry[K, V]).value = value
		c.order.MoveToBack(el)
		return
	}
	if c.order.Len() >= c.cap {
		if front := c.order.Front(); front != nil {
			c.order.Remove(front)
			delete(c.items, front.Value.(*lruEntry[K, V]).key)
		}
	}
	el := c.order.PushBack(&lruEntry[K, V]{key: key, value: value})
	c.items[key] = el
}

// snapshot returns a copy of all entries (used for DB persistence).
func (c *lruCache[K, V]) snapshot() []lruEntry[K, V] {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]lruEntry[K, V], 0, c.order.Len())
	for el := c.order.Front(); el != nil; el = el.Next() {
		out = append(out, *el.Value.(*lruEntry[K, V]))
	}
	return out
}

// evictExact removes a single entry by exact key from any lruCache.
// Must be called without c.mu held.
func (c *lruCache[K, V]) evictExact(key K) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.order.Remove(el)
		delete(c.items, key)
	}
}

// ── Token-bucket rate limiter ─────────────────────────────────────────────────

type tokenBucket struct {
	tokens    float64
	lastRefil time.Time
}

// allow returns true if the request should be permitted and deducts one token.
func (tb *tokenBucket) allow() bool {
	now := time.Now()
	elapsed := now.Sub(tb.lastRefil).Seconds()
	tb.tokens += elapsed * (float64(rateLimitPerMin) / 60.0)
	if tb.tokens > float64(rateLimitBurst) {
		tb.tokens = float64(rateLimitBurst)
	}
	tb.lastRefil = now
	if tb.tokens < 1 {
		return false
	}
	tb.tokens--
	return true
}

// ── ML settings ───────────────────────────────────────────────────────────────

// mlSettings is an immutable snapshot of all ML tuning knobs.
// Replacing the pointer atomically avoids taking separate locks in the hot DNS path.
type mlSettings struct {
	threshold     float32
	blockDGA      bool
	blockPhishing bool
	blockMalware  bool
	blockOther    bool
}

// ── Service schedule cache ────────────────────────────────────────────────────

type svcScheduleEntry struct {
	found    bool
	enabled  int
	daysCSV  string
	tStart   string
	tEnd     string
	cachedAt time.Time
}

type svcScheduleKey struct {
	scope    string
	scopeKey string
	svcID    string
}

// ── GuardianServer ────────────────────────────────────────────────────────────

type GuardianServer struct {
	// Configuration
	upstreams   []string
	upstreamMu  sync.RWMutex
	mlAddress   string
	blockFile   string
	blackholeA  net.IP
	blackholeAA net.IP
	logLevel    LogLevel

	// Feature toggles — atomic int32 (0=false, 1=true) for lock-free reads.
	mlEnabledAtomic        int32
	blocklistEnabledAtomic int32

	// ML settings — single atomic pointer avoids separate per-field mutexes.
	mlSettingsAtomic atomic.Pointer[mlSettings]

	// Runtime
	blocklist map[string]struct{}
	blMu      sync.RWMutex

	db      *sql.DB
	dbMu    sync.Mutex // guards write operations; WAL allows concurrent reads
	logStmt *sql.Stmt  // prepared INSERT for query logging, reused per DNS query

	mlConn   *grpc.ClientConn
	mlCli    guardian.GuardianAIClient
	mlConnMu sync.RWMutex // guards mlConn and mlCli

	reloadMu sync.Mutex // serialises concurrent reloadAllSources calls

	// O(1) LRU caches
	mlCache  *lruCache[string, mlCacheEntry]
	dnsCache *lruCache[string, dnsCacheEntry]

	// Service schedule cache — avoids a DB hit on every DNS query.
	svcSchedCache   map[svcScheduleKey]svcScheduleEntry
	svcSchedCacheMu sync.Mutex

	// Per-client token-bucket rate limiters.
	rateLimit   map[string]*tokenBucket
	rateLimitMu sync.Mutex
}

type mlCacheEntry struct {
	isMalicious bool
	category    string
	confidence  float32
	expiresAt   time.Time
}

// ── Constructor ───────────────────────────────────────────────────────────────

func NewGuardianServer(upstream, mlAddr, blockfile, dbPath string, logLevel LogLevel) (*GuardianServer, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return nil, fmt.Errorf("enable WAL mode: %w", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)

	upstreamList := parseUpstreamServers(upstream)
	if len(upstreamList) == 0 {
		return nil, fmt.Errorf("at least one upstream DNS server must be provided")
	}

	defaultML := &mlSettings{
		threshold:     0.9,
		blockDGA:      true,
		blockPhishing: true,
		blockMalware:  true,
		blockOther:    true,
	}

	s := &GuardianServer{
		upstreams:     upstreamList,
		mlAddress:     mlAddr,
		blockFile:     blockfile,
		blackholeA:    net.ParseIP("0.0.0.0"),
		blackholeAA:   net.ParseIP("::1"),
		logLevel:      logLevel,
		blocklist:     make(map[string]struct{}),
		db:            db,
		mlCache:       newLRUCache[string, mlCacheEntry](maxMLCacheSize),
		dnsCache:      newLRUCache[string, dnsCacheEntry](maxDNSCacheSize),
		svcSchedCache: make(map[svcScheduleKey]svcScheduleEntry),
		rateLimit:     make(map[string]*tokenBucket),
	}
	atomic.StoreInt32(&s.mlEnabledAtomic, 1)
	atomic.StoreInt32(&s.blocklistEnabledAtomic, 1)
	s.mlSettingsAtomic.Store(defaultML)

	s.startBackgroundTasks(mlAddr)

	if err := s.initDB(); err != nil {
		return nil, err
	}

	s.logStmt, err = db.Prepare(
		"INSERT INTO queries (timestamp, domain, qtype, client_ip, blocked, category, confidence, reason) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
	)
	if err != nil {
		return nil, fmt.Errorf("prepare logStmt: %w", err)
	}

	s.loadUpstreamsFromDB()
	s.loadTogglesFromDB()
	_ = s.loadBlocklistFromFile()
	go s.reloadAllSources()
	go s.applyCustomRulesFromDB()

	s.loadDNSCacheFromDB()

	return s, nil
}

// startBackgroundTasks launches all long-running goroutines for the server.
func (s *GuardianServer) startBackgroundTasks(mlAddr string) {
	// Prune stale token buckets every minute.
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			cutoff := time.Now().Add(-2 * time.Minute)
			s.rateLimitMu.Lock()
			for ip, tb := range s.rateLimit {
				if tb.lastRefil.Before(cutoff) {
					delete(s.rateLimit, ip)
				}
			}
			s.rateLimitMu.Unlock()
		}
	}()

	// Prune expired sessions every hour.
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			s.dbMu.Lock()
			_, _ = s.db.Exec("DELETE FROM sessions WHERE expires_at < ?", time.Now().UTC().Format(time.RFC3339))
			s.dbMu.Unlock()
		}
	}()

	// Prune old query-log rows daily, keeping the most recent 500 000 entries,
	// then checkpoint the WAL so reclaimed space is returned to the OS.
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			s.dbMu.Lock()
			_, _ = s.db.Exec(`
				DELETE FROM queries
				WHERE id NOT IN (
					SELECT id FROM queries ORDER BY id DESC LIMIT 500000
				)`)
			_, _ = s.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
			s.dbMu.Unlock()
			s.log(LogLevelInfo, "db", map[string]any{"action": "pruned_queries"})
		}
	}()

	if mlAddr == "" {
		return
	}

	// ML reconnect loop — recovers automatically if the Python process restarts.
	go func() {
		for {
			s.mlConnMu.RLock()
			needConnect := s.mlCli == nil
			s.mlConnMu.RUnlock()

			if needConnect {
				conn, err := grpc.Dial(mlAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
				if err != nil {
					s.log(LogLevelWarn, "ml", map[string]any{"action": "reconnect_failed", "addr": mlAddr, "error": err.Error()})
				} else {
					cli := guardian.NewGuardianAIClient(conn)
					ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
					_, probeErr := cli.PredictDomain(ctx, &guardian.DomainRequest{Domain: "probe.internal"})
					cancel()
					if probeErr == nil || strings.Contains(probeErr.Error(), "desc =") {
						s.mlConnMu.Lock()
						if s.mlConn != nil {
							_ = s.mlConn.Close()
						}
						s.mlConn = conn
						s.mlCli = cli
						s.mlConnMu.Unlock()
						s.log(LogLevelInfo, "ml", map[string]any{"action": "connected", "addr": mlAddr})
					} else {
						_ = conn.Close()
						s.log(LogLevelDebug, "ml", map[string]any{"action": "probe_failed", "error": probeErr.Error()})
					}
				}
			} else {
				// Already connected — probe liveness every 10 s.
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				s.mlConnMu.RLock()
				cli := s.mlCli
				s.mlConnMu.RUnlock()
				_, probeErr := cli.PredictDomain(ctx, &guardian.DomainRequest{Domain: "probe.internal"})
				cancel()
				if probeErr != nil && !strings.Contains(probeErr.Error(), "desc =") {
					s.mlConnMu.Lock()
					s.mlCli = nil
					s.mlConnMu.Unlock()
					s.log(LogLevelWarn, "ml", map[string]any{"action": "disconnected", "error": probeErr.Error()})
				}
			}

			time.Sleep(10 * time.Second)
		}
	}()
}

// ── Feature-toggle helpers ────────────────────────────────────────────────────

func (s *GuardianServer) mlEnabled() bool {
	return atomic.LoadInt32(&s.mlEnabledAtomic) == 1
}

func (s *GuardianServer) blocklistEnabled() bool {
	return atomic.LoadInt32(&s.blocklistEnabledAtomic) == 1
}

// ── DNS cache persistence ─────────────────────────────────────────────────────

// saveDNSCacheToDB persists all non-expired DNS cache entries to the
// dns_cache table so they survive a server restart.
func (s *GuardianServer) saveDNSCacheToDB() {
	type row struct {
		key       string
		msgBytes  []byte
		expiresAt time.Time
	}

	now := time.Now()
	entries := s.dnsCache.snapshot()
	rows := make([]row, 0, len(entries))
	for _, ent := range entries {
		if ent.value.expiresAt.After(now) {
			if b, err := ent.value.msg.Pack(); err == nil {
				rows = append(rows, row{ent.key, b, ent.value.expiresAt})
			}
		}
	}

	s.dbMu.Lock()
	defer s.dbMu.Unlock()
	_, _ = s.db.Exec(`DELETE FROM dns_cache`)
	for _, r := range rows {
		_, _ = s.db.Exec(
			`INSERT INTO dns_cache (cache_key, msg_bytes, expires_at) VALUES (?, ?, ?)`,
			r.key, r.msgBytes, r.expiresAt.UTC().Format(time.RFC3339),
		)
	}
}

// loadDNSCacheFromDB restores previously persisted DNS cache entries,
// discarding any that have already expired.
func (s *GuardianServer) loadDNSCacheFromDB() {
	rows, err := s.db.Query(`SELECT cache_key, msg_bytes, expires_at FROM dns_cache`)
	if err != nil {
		return
	}
	defer rows.Close()

	now := time.Now()
	for rows.Next() {
		var key, expiresStr string
		var msgBytes []byte
		if err := rows.Scan(&key, &msgBytes, &expiresStr); err != nil {
			continue
		}
		expiresAt, err := time.Parse(time.RFC3339, expiresStr)
		if err != nil || expiresAt.Before(now) {
			continue
		}
		msg := new(dns.Msg)
		if err := msg.Unpack(msgBytes); err != nil {
			continue
		}
		s.dnsCache.set(key, dnsCacheEntry{msg: msg, expiresAt: expiresAt})
	}
}

// ── Database initialisation ───────────────────────────────────────────────────

func (s *GuardianServer) initDB() error {
	const schema = `
	CREATE TABLE IF NOT EXISTS users (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		username      TEXT    NOT NULL UNIQUE,
		password_hash BLOB    NOT NULL
	);
	CREATE TABLE IF NOT EXISTS sessions (
		token      TEXT    PRIMARY KEY,
		user_id    INTEGER,
		expires_at DATETIME
	);
	CREATE TABLE IF NOT EXISTS queries (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp  DATETIME,
		domain     TEXT,
		qtype      INTEGER,
		client_ip  TEXT,
		blocked    INTEGER,
		category   TEXT,
		confidence REAL
	);
	CREATE INDEX IF NOT EXISTS idx_queries_timestamp      ON queries(timestamp DESC);
	CREATE INDEX IF NOT EXISTS idx_queries_blocked_ts     ON queries(blocked, timestamp DESC);
	CREATE INDEX IF NOT EXISTS idx_queries_client_ip      ON queries(client_ip);
	CREATE INDEX IF NOT EXISTS idx_queries_domain         ON queries(domain);
	CREATE INDEX IF NOT EXISTS idx_queries_domain_blocked ON queries(domain, blocked);
	CREATE INDEX IF NOT EXISTS idx_queries_domain_ts      ON queries(domain, timestamp DESC);
	CREATE TABLE IF NOT EXISTS dns_cache (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		cache_key  TEXT    NOT NULL UNIQUE,
		msg_bytes  BLOB    NOT NULL,
		expires_at TEXT    NOT NULL
	);
	CREATE TABLE IF NOT EXISTS blocklist_sources (
		id   INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT    NOT NULL,
		url  TEXT    NOT NULL UNIQUE
	);
	CREATE TABLE IF NOT EXISTS custom_rules (
		id    INTEGER PRIMARY KEY AUTOINCREMENT,
		rules TEXT    NOT NULL DEFAULT ''
	);
	CREATE TABLE IF NOT EXISTS settings (
		key   TEXT PRIMARY KEY,
		value TEXT NOT NULL DEFAULT ''
	);
	CREATE TABLE IF NOT EXISTS client_rules (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		client_ip  TEXT    NOT NULL UNIQUE,
		label      TEXT    NOT NULL DEFAULT '',
		blocked    INTEGER NOT NULL DEFAULT 0,
		rules      TEXT    NOT NULL DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS ml_feedback (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		domain     TEXT    NOT NULL,
		verdict    TEXT    NOT NULL CHECK(verdict IN ('safe','malicious')),
		category   TEXT    NOT NULL DEFAULT '',
		confidence REAL    NOT NULL DEFAULT 0,
		client_ip  TEXT    NOT NULL DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS service_schedules (
		id           INTEGER PRIMARY KEY AUTOINCREMENT,
		scope        TEXT    NOT NULL DEFAULT 'global',
		scope_key    TEXT    NOT NULL DEFAULT '',
		service_id   TEXT    NOT NULL,
		enabled      INTEGER NOT NULL DEFAULT 0,
		days_of_week TEXT    NOT NULL DEFAULT '',
		time_start   TEXT    NOT NULL DEFAULT '',
		time_end     TEXT    NOT NULL DEFAULT '',
		UNIQUE(scope, scope_key, service_id)
	);
	CREATE TABLE IF NOT EXISTS client_groups (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		name       TEXT    NOT NULL,
		label      TEXT    NOT NULL DEFAULT '',
		blocked    INTEGER NOT NULL DEFAULT 0,
		rules      TEXT    NOT NULL DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS client_group_members (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		group_id   INTEGER NOT NULL REFERENCES client_groups(id) ON DELETE CASCADE,
		identifier TEXT    NOT NULL,
		type       TEXT    NOT NULL DEFAULT 'ip',
		UNIQUE(group_id, identifier)
	);
	`
	if _, err := s.db.Exec(schema); err != nil {
		return err
	}

	if err := s.migrateClientRules(); err != nil {
		return err
	}

	// Seed default blocklist source on first run (no-op on subsequent starts).
	if _, err := s.db.Exec(
		"INSERT OR IGNORE INTO blocklist_sources (name, url) VALUES (?, ?)",
		"AdGuard DNS filter",
		"https://adguardteam.github.io/HostlistsRegistry/assets/filter_1.txt",
	); err != nil {
		return err
	}

	// Add `reason` column to queries if it does not exist yet.
	var hasReason int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('queries') WHERE name='reason'`).Scan(&hasReason)
	if hasReason == 0 {
		_, _ = s.db.Exec(`ALTER TABLE queries ADD COLUMN reason TEXT NOT NULL DEFAULT ''`)
		_, _ = s.db.Exec(`UPDATE queries SET reason = category WHERE category != ''`)
	}

	// Seed default query retention setting.
	_, _ = s.db.Exec(`INSERT OR IGNORE INTO settings (key, value) VALUES ('queries_retain_days', '90')`)

	// Prune old rows immediately on startup according to the stored retention setting.
	var retainVal string
	_ = s.db.QueryRow(`SELECT value FROM settings WHERE key='queries_retain_days'`).Scan(&retainVal)
	retainDays := 90
	if v, err := strconv.Atoi(retainVal); err == nil && v > 0 {
		retainDays = v
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -retainDays).Format(time.RFC3339)
	_, _ = s.db.Exec(`DELETE FROM queries WHERE timestamp < ?`, cutoff)

	return nil
}

// migrateClientRules handles the one-time migration of the legacy
// allow_list + block_list columns to the unified rules column.
func (s *GuardianServer) migrateClientRules() error {
	var hasAllowList int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('client_rules') WHERE name='allow_list'`).Scan(&hasAllowList)
	if hasAllowList == 0 {
		return nil
	}

	_, _ = s.db.Exec(`
		CREATE TABLE IF NOT EXISTS client_rules_new (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			client_ip  TEXT    NOT NULL UNIQUE,
			label      TEXT    NOT NULL DEFAULT '',
			blocked    INTEGER NOT NULL DEFAULT 0,
			rules      TEXT    NOT NULL DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`)

	rows, err := s.db.Query(`SELECT client_ip, label, blocked, allow_list, block_list, created_at FROM client_rules`)
	if err != nil {
		return nil // non-fatal; old table may already be gone
	}
	defer rows.Close()
	for rows.Next() {
		var ip, label, allowList, blockList, createdAt string
		var blocked int
		if rows.Scan(&ip, &label, &blocked, &allowList, &blockList, &createdAt) != nil {
			continue
		}
		var combined strings.Builder
		for _, ln := range strings.Split(allowList, "\n") {
			if d := strings.TrimSpace(ln); d != "" {
				combined.WriteString("@@||" + d + "^\n")
			}
		}
		for _, ln := range strings.Split(blockList, "\n") {
			if d := strings.TrimSpace(ln); d != "" {
				combined.WriteString("||" + d + "^\n")
			}
		}
		_, _ = s.db.Exec(
			`INSERT OR IGNORE INTO client_rules_new (client_ip, label, blocked, rules, created_at) VALUES (?, ?, ?, ?, ?)`,
			ip, label, blocked, strings.TrimSpace(combined.String()), createdAt,
		)
	}
	_, _ = s.db.Exec(`DROP TABLE client_rules`)
	_, _ = s.db.Exec(`ALTER TABLE client_rules_new RENAME TO client_rules`)
	return nil
}

// ── Settings persistence ──────────────────────────────────────────────────────

func (s *GuardianServer) loadUpstreamsFromDB() {
	var val string
	if err := s.db.QueryRow("SELECT value FROM settings WHERE key = 'upstream_servers'").Scan(&val); err != nil {
		return
	}
	if strings.TrimSpace(val) == "" {
		return
	}
	if parsed := parseUpstreamServers(val); len(parsed) > 0 {
		s.upstreamMu.Lock()
		s.upstreams = parsed
		s.upstreamMu.Unlock()
		s.log(LogLevelInfo, "upstream", map[string]any{"action": "loaded_from_db", "count": len(parsed)})
	}
}

func (s *GuardianServer) saveUpstreamsToDB(servers []string) {
	val := strings.Join(servers, "\n")
	s.dbMu.Lock()
	defer s.dbMu.Unlock()
	_, _ = s.db.Exec(
		"INSERT INTO settings (key, value) VALUES ('upstream_servers', ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value",
		val,
	)
}

func (s *GuardianServer) loadTogglesFromDB() {
	var mlVal, blVal string
	var threshVal, dgaVal, phishVal, malVal, otherVal string
	_ = s.db.QueryRow("SELECT value FROM settings WHERE key = 'ml_enabled'").Scan(&mlVal)
	_ = s.db.QueryRow("SELECT value FROM settings WHERE key = 'blocklist_enabled'").Scan(&blVal)
	_ = s.db.QueryRow("SELECT value FROM settings WHERE key = 'ml_threshold'").Scan(&threshVal)
	_ = s.db.QueryRow("SELECT value FROM settings WHERE key = 'ml_block_dga'").Scan(&dgaVal)
	_ = s.db.QueryRow("SELECT value FROM settings WHERE key = 'ml_block_phishing'").Scan(&phishVal)
	_ = s.db.QueryRow("SELECT value FROM settings WHERE key = 'ml_block_malware'").Scan(&malVal)
	_ = s.db.QueryRow("SELECT value FROM settings WHERE key = 'ml_block_other'").Scan(&otherVal)

	parseBool := func(raw string) (int32, bool) {
		if raw == "" {
			return 0, false
		}
		if raw == "false" {
			return 0, true
		}
		return 1, true
	}
	if v, ok := parseBool(mlVal); ok {
		atomic.StoreInt32(&s.mlEnabledAtomic, v)
	}
	if v, ok := parseBool(blVal); ok {
		atomic.StoreInt32(&s.blocklistEnabledAtomic, v)
	}

	cur := s.mlSettingsAtomic.Load()
	next := *cur
	if threshVal != "" {
		if v, err := strconv.ParseFloat(threshVal, 32); err == nil && v > 0 && v <= 1 {
			next.threshold = float32(v)
		}
	}
	if dgaVal != "" {
		next.blockDGA = dgaVal != "false"
	}
	if phishVal != "" {
		next.blockPhishing = phishVal != "false"
	}
	if malVal != "" {
		next.blockMalware = malVal != "false"
	}
	if otherVal != "" {
		next.blockOther = otherVal != "false"
	}
	s.mlSettingsAtomic.Store(&next)
}

func (s *GuardianServer) saveToggleToDB(key string, val bool) {
	v := "true"
	if !val {
		v = "false"
	}
	s.dbMu.Lock()
	defer s.dbMu.Unlock()
	_, _ = s.db.Exec(
		"INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value",
		key, v,
	)
}

// ── Blocklist file I/O ────────────────────────────────────────────────────────

func (s *GuardianServer) loadBlocklistFromFile() error {
	if s.blockFile == "" {
		return nil
	}
	data, err := os.ReadFile(s.blockFile)
	if err != nil {
		return err
	}
	newBL := make(map[string]struct{})
	for _, ln := range strings.Split(string(data), "\n") {
		if d := parseBlocklistLine(ln); d != "" {
			newBL[d] = struct{}{}
		}
	}
	s.blMu.Lock()
	s.blocklist = newBL
	s.blMu.Unlock()
	s.log(LogLevelInfo, "blocklist", map[string]any{"action": "loaded", "entries": len(newBL), "file": s.blockFile})
	return nil
}

func (s *GuardianServer) persistBlocklistToFile() error {
	s.blMu.RLock()
	items := make([]string, 0, len(s.blocklist))
	for d := range s.blocklist {
		items = append(items, d)
	}
	s.blMu.RUnlock()

	sort.Strings(items)
	if dir := filepath.Dir(s.blockFile); dir != "" && dir != "." {
		_ = os.MkdirAll(dir, 0o755)
	}
	f, err := os.Create(s.blockFile)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, d := range items {
		_, _ = f.WriteString("0.0.0.0 " + d + "\n")
	}
	return nil
}

// ── Service domain helpers ────────────────────────────────────────────────────

// serviceIDsForDomain returns all service IDs whose domain list matches the
// given DNS query name (exact or parent-domain match).
func serviceIDsForDomain(qname string) []string {
	qname = strings.ToLower(strings.TrimSuffix(qname, "."))
	seen := map[string]struct{}{}
	var ids []string
	parts := strings.Split(qname, ".")
	for i := 0; i < len(parts)-1; i++ {
		candidate := strings.Join(parts[i:], ".")
		for _, id := range serviceDomainIndex[candidate] {
			if _, already := seen[id]; !already {
				seen[id] = struct{}{}
				ids = append(ids, id)
			}
		}
	}
	return ids
}

// checkServiceBlock returns true if a predefined-service block rule fires for
// the given domain + scopeKey combination at the current wall-clock time.
//
// scopeKey is "group:<id>" when the client belongs to a group, or the raw
// client IP otherwise. Per-client rules take priority over global rules:
//
//   - Row exists for (client, scopeKey, svcID), enabled=0 → explicitly unblocked; skip.
//   - Row exists for (client, scopeKey, svcID), enabled=1 → apply time window.
//   - No client row → fall back to (global, "", svcID).
//   - No row at either scope → not blocked.
func (s *GuardianServer) checkServiceBlock(domain, scopeKey string) bool {
	svcIDs := serviceIDsForDomain(domain)
	if len(svcIDs) == 0 {
		return false
	}
	now := time.Now()
	weekday := int(now.Weekday())
	hhmm := now.Format("15:04")

	withinWindow := func(daysCSV, tStart, tEnd string) bool {
		dayMatch := true
		if daysCSV != "" {
			dayMatch = false
			for _, part := range strings.Split(daysCSV, ",") {
				if d, err := strconv.Atoi(strings.TrimSpace(part)); err == nil && d == weekday {
					dayMatch = true
					break
				}
			}
		}
		timeMatch := tStart == "" || tEnd == "" || (hhmm >= tStart && hhmm <= tEnd)
		return dayMatch && timeMatch
	}

	queryRow := func(scope, key, svcID string) (found bool, enabled int, daysCSV, tStart, tEnd string) {
		cacheKey := svcScheduleKey{scope: scope, scopeKey: key, svcID: svcID}
		now := time.Now()
		s.svcSchedCacheMu.Lock()
		if e, ok := s.svcSchedCache[cacheKey]; ok && now.Sub(e.cachedAt) < svcScheduleCacheTTL {
			s.svcSchedCacheMu.Unlock()
			return e.found, e.enabled, e.daysCSV, e.tStart, e.tEnd
		}
		s.svcSchedCacheMu.Unlock()

		var en int
		var dc, ts, te string
		err := s.db.QueryRow(
			`SELECT enabled, days_of_week, time_start, time_end
			   FROM service_schedules
			  WHERE scope=? AND scope_key=? AND service_id=?`,
			scope, key, svcID,
		).Scan(&en, &dc, &ts, &te)
		entry := svcScheduleEntry{found: err == nil, enabled: en, daysCSV: dc, tStart: ts, tEnd: te, cachedAt: now}
		s.svcSchedCacheMu.Lock()
		s.svcSchedCache[cacheKey] = entry
		s.svcSchedCacheMu.Unlock()
		return entry.found, entry.enabled, entry.daysCSV, entry.tStart, entry.tEnd
	}

	for _, svcID := range svcIDs {
		if scopeKey != "" {
			if found, enabled, daysCSV, tStart, tEnd := queryRow("client", scopeKey, svcID); found {
				if enabled == 0 {
					continue // explicitly unblocked at client level
				}
				if withinWindow(daysCSV, tStart, tEnd) {
					return true
				}
				continue // client row is authoritative; don't fall through to global
			}
		}
		if found, enabled, daysCSV, tStart, tEnd := queryRow("global", "", svcID); found {
			if enabled == 0 {
				continue
			}
			if withinWindow(daysCSV, tStart, tEnd) {
				return true
			}
		}
	}
	return false
}

// evictDNSByPrefix removes all DNS cache entries whose key starts with prefix.
func (s *GuardianServer) evictDNSByPrefix(prefix string) {
	s.dnsCache.mu.Lock()
	defer s.dnsCache.mu.Unlock()
	for el := s.dnsCache.order.Front(); el != nil; {
		next := el.Next()
		entry := el.Value.(*lruEntry[string, dnsCacheEntry])
		if strings.HasPrefix(entry.key, prefix) {
			s.dnsCache.order.Remove(el)
			delete(s.dnsCache.items, entry.key)
		}
		el = next
	}
}

// flushServiceDNSCache evicts all DNS cache entries for every domain in the
// named service. Should be called after any schedule change for that service.
func (s *GuardianServer) flushServiceDNSCache(serviceID string) {
	for _, svc := range PredefinedServices {
		if svc.ID != serviceID {
			continue
		}
		for _, d := range svc.Domains {
			s.evictDNSByPrefix(strings.ToLower(d) + ":")
		}
		break
	}
}

// flushServiceSchedCache evicts all schedule-cache entries for the named service.
func (s *GuardianServer) flushServiceSchedCache(serviceID string) {
	s.svcSchedCacheMu.Lock()
	defer s.svcSchedCacheMu.Unlock()
	for k := range s.svcSchedCache {
		if k.svcID == serviceID {
			delete(s.svcSchedCache, k)
		}
	}
}

// ── Blocklist helpers ─────────────────────────────────────────────────────────

// parseClientRules evaluates AdGuard-syntax per-client rules against qname.
// Returns ("block"|"allow", true) on a match, or ("", false) otherwise.
//
// Supported patterns:
//
//	@@||domain^   — allow (allowlist)
//	||domain^     — block
//	@@domain      — allow plain domain
//	domain        — block plain domain
//	# or ! lines  — comments, ignored
func parseClientRules(rules, domain string) (action string, matched bool) {
	domain = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(domain), "."))
	for _, raw := range strings.Split(rules, "\n") {
		ln := strings.TrimSpace(raw)
		if ln == "" || strings.HasPrefix(ln, "#") || strings.HasPrefix(ln, "!") {
			continue
		}
		isAllow := strings.HasPrefix(ln, "@@")
		if isAllow {
			ln = strings.TrimPrefix(ln, "@@")
		}
		if strings.HasPrefix(ln, "||") {
			ln = strings.TrimPrefix(ln, "||")
			if idx := strings.IndexAny(ln, "^$"); idx != -1 {
				ln = ln[:idx]
			}
		}
		ln = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(ln), "."))
		if ln == "" {
			continue
		}
		if domain == ln || strings.HasSuffix(domain, "."+ln) {
			if isAllow {
				return "allow", true
			}
			return "block", true
		}
	}
	return "", false
}

// parseBlocklistLine extracts a domain from a single line in any supported
// hosts/adblock format. Returns empty string if the line should be skipped.
func parseBlocklistLine(ln string) string {
	ln = strings.TrimSpace(ln)
	if ln == "" || strings.HasPrefix(ln, "#") || strings.HasPrefix(ln, "!") ||
		strings.HasPrefix(ln, "@@") || (strings.HasPrefix(ln, "/") && strings.HasSuffix(ln, "/")) {
		return ""
	}
	var domain string
	if strings.HasPrefix(ln, "||") {
		d := strings.TrimPrefix(ln, "||")
		if idx := strings.IndexAny(d, "^$"); idx != -1 {
			d = d[:idx]
		}
		domain = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(d), "."))
	} else {
		parts := strings.Fields(ln)
		switch {
		case len(parts) == 1:
			domain = strings.ToLower(strings.TrimSuffix(parts[0], "."))
		case len(parts) >= 2 && net.ParseIP(parts[0]) != nil:
			domain = strings.ToLower(strings.TrimSuffix(parts[1], "."))
		case len(parts) >= 2:
			domain = strings.ToLower(strings.TrimSuffix(parts[0], "."))
		}
	}
	if domain != "" && strings.Contains(domain, ".") && !strings.Contains(domain, " ") {
		return domain
	}
	return ""
}

// reloadAllSources clears the in-memory blocklist and re-fetches every URL
// stored in blocklist_sources, then persists the result to disk.
// reloadMu ensures only one reload runs at a time.
func (s *GuardianServer) reloadAllSources() {
	s.reloadMu.Lock()
	defer s.reloadMu.Unlock()

	rows, err := s.db.Query("SELECT url FROM blocklist_sources")
	if err != nil {
		s.log(LogLevelWarn, "blocklist", map[string]any{"action": "reload_sources", "error": err.Error()})
		return
	}
	var urls []string
	for rows.Next() {
		var u string
		if rows.Scan(&u) == nil {
			urls = append(urls, u)
		}
	}
	rows.Close()

	if len(urls) == 0 {
		s.blMu.Lock()
		s.blocklist = make(map[string]struct{})
		s.blMu.Unlock()
		s.applyCustomRulesFromDB()
		_ = s.persistBlocklistToFile()
		s.log(LogLevelInfo, "blocklist", map[string]any{"action": "reloaded", "total": 0, "sources": 0})
		return
	}

	type fetchResult struct {
		url     string
		domains []string
		err     error
	}
	results := make(chan fetchResult, len(urls))
	httpClient := &http.Client{Timeout: 60 * time.Second}
	for _, u := range urls {
		u := u
		go func() {
			resp, err := httpClient.Get(u)
			if err != nil {
				results <- fetchResult{url: u, err: err}
				return
			}
			data, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				results <- fetchResult{url: u, err: err}
				return
			}
			var domains []string
			for _, ln := range strings.Split(string(data), "\n") {
				if d := parseBlocklistLine(ln); d != "" {
					domains = append(domains, d)
				}
			}
			results <- fetchResult{url: u, domains: domains}
		}()
	}

	newBL := make(map[string]struct{})
	for range urls {
		r := <-results
		if r.err != nil {
			s.log(LogLevelWarn, "blocklist", map[string]any{"action": "fetch", "url": r.url, "error": r.err.Error()})
			continue
		}
		added := 0
		for _, d := range r.domains {
			if _, exists := newBL[d]; !exists {
				newBL[d] = struct{}{}
				added++
			}
		}
		s.log(LogLevelInfo, "blocklist", map[string]any{"action": "fetched", "url": r.url, "added": added})
	}

	s.blMu.Lock()
	s.blocklist = newBL
	s.blMu.Unlock()
	s.applyCustomRulesFromDB()
	_ = s.persistBlocklistToFile()
	s.log(LogLevelInfo, "blocklist", map[string]any{"action": "reloaded", "total": len(newBL), "sources": len(urls)})
}

// applyCustomRulesFromDB merges saved custom rules into the in-memory blocklist.
func (s *GuardianServer) applyCustomRulesFromDB() {
	var rules string
	_ = s.db.QueryRow("SELECT rules FROM custom_rules ORDER BY id DESC LIMIT 1").Scan(&rules)
	if rules == "" {
		return
	}
	s.blMu.Lock()
	for _, ln := range strings.Split(rules, "\n") {
		if d := parseBlocklistLine(ln); d != "" {
			s.blocklist[d] = struct{}{}
		}
	}
	s.blMu.Unlock()
	s.log(LogLevelInfo, "blocklist", map[string]any{"action": "custom_rules_applied"})
}

func (s *GuardianServer) blocklistHas(domain string) bool {
	domain = strings.TrimSuffix(strings.ToLower(domain), ".")
	s.blMu.RLock()
	defer s.blMu.RUnlock()
	if _, ok := s.blocklist[domain]; ok {
		return true
	}
	parts := strings.Split(domain, ".")
	for i := 1; i < len(parts); i++ {
		if _, ok := s.blocklist[strings.Join(parts[i:], ".")]; ok {
			return true
		}
	}
	return false
}

func (s *GuardianServer) blocklistAdd(domain string) {
	domain = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(domain)), ".")
	if domain == "" {
		return
	}
	s.blMu.Lock()
	s.blocklist[domain] = struct{}{}
	s.blMu.Unlock()
	go func() { _ = s.persistBlocklistToFile() }()
}

func (s *GuardianServer) blocklistRemove(domain string) {
	domain = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(domain)), ".")
	if domain == "" {
		return
	}
	s.blMu.Lock()
	delete(s.blocklist, domain)
	s.blMu.Unlock()
	go func() { _ = s.persistBlocklistToFile() }()
}

// ── ML classification ─────────────────────────────────────────────────────────

func (s *GuardianServer) classifyDomain(domain string) (isMalicious bool, category string, confidence float32, err error) {
	domain = strings.TrimSpace(strings.ToLower(domain))
	if e, ok := s.mlCache.get(domain); ok && time.Now().Before(e.expiresAt) {
		return e.isMalicious, e.category, e.confidence, nil
	}

	s.mlConnMu.RLock()
	cli := s.mlCli
	s.mlConnMu.RUnlock()
	if cli == nil {
		return false, "unknown", 0, fmt.Errorf("ml not connected")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, err := cli.PredictDomain(ctx, &guardian.DomainRequest{Domain: domain})
	if err != nil {
		s.mlConnMu.Lock()
		s.mlCli = nil
		s.mlConnMu.Unlock()
		return false, "unknown", 0, err
	}

	isMal := resp.GetIsMalicious()
	cat := resp.GetCategory()
	conf := resp.GetConfidenceScore()
	s.mlCache.set(domain, mlCacheEntry{
		isMalicious: isMal,
		category:    cat,
		confidence:  conf,
		expiresAt:   time.Now().Add(mlCacheTTL),
	})
	return isMal, cat, conf, nil
}

// applyMLCategoryFilter applies per-category toggles and the confidence
// threshold to an ML result. Returns true if the domain should be blocked.
func (s *GuardianServer) applyMLCategoryFilter(isMal bool, cat string, conf float32) bool {
	if !isMal {
		return false
	}
	ml := s.mlSettingsAtomic.Load()
	lower := strings.ToLower(cat)
	isDGA := strings.Contains(lower, "dga")
	isPhishing := strings.Contains(lower, "phishing")
	isMalware := strings.Contains(lower, "malware")
	categoryAllowed := (isDGA && ml.blockDGA) ||
		(isPhishing && ml.blockPhishing) ||
		(isMalware && ml.blockMalware) ||
		(!isDGA && !isPhishing && !isMalware && ml.blockOther)
	return conf >= ml.threshold && categoryAllowed
}

// ── DNS handler ───────────────────────────────────────────────────────────────

// blackholeReply builds a DNS sinkhole response for the given request.
// A → 0.0.0.0, AAAA → ::1, everything else → NXDOMAIN.
func (s *GuardianServer) blackholeReply(req *dns.Msg, ttl uint32) *dns.Msg {
	q := req.Question[0]
	m := new(dns.Msg)
	m.SetReply(req)
	m.Authoritative = true
	switch q.Qtype {
	case dns.TypeA:
		m.Answer = []dns.RR{&dns.A{
			Hdr: dns.RR_Header{Name: q.Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: ttl},
			A:   s.blackholeA,
		}}
	case dns.TypeAAAA:
		m.Answer = []dns.RR{&dns.AAAA{
			Hdr:  dns.RR_Header{Name: q.Name, Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: ttl},
			AAAA: s.blackholeAA,
		}}
	default:
		m.Rcode = dns.RcodeNameError
	}
	return m
}

func (s *GuardianServer) handleDNSQuery(w dns.ResponseWriter, req *dns.Msg) {
	if len(req.Question) == 0 {
		m := new(dns.Msg)
		m.SetReply(req)
		w.WriteMsg(m)
		return
	}
	qname := strings.TrimSuffix(req.Question[0].Name, ".")
	qtype := req.Question[0].Qtype

	// ── Resolve client IP ────────────────────────────────────────────────────
	var clientIP string
	switch addr := w.RemoteAddr().(type) {
	case *net.UDPAddr:
		clientIP = addr.IP.String()
	case *net.TCPAddr:
		clientIP = addr.IP.String()
	}
	if clientIP == "" {
		clientIP = "unknown"
	}

	// ── Rate limiting ────────────────────────────────────────────────────────
	if clientIP != "unknown" {
		s.rateLimitMu.Lock()
		tb, exists := s.rateLimit[clientIP]
		if !exists {
			tb = &tokenBucket{tokens: float64(rateLimitBurst), lastRefil: time.Now()}
			s.rateLimit[clientIP] = tb
		}
		allowed := tb.allow()
		s.rateLimitMu.Unlock()
		if !allowed {
			s.log(LogLevelWarn, "rate", map[string]any{"action": "limit_exceeded", "client": clientIP})
			m := new(dns.Msg)
			m.SetRcode(req, dns.RcodeRefused)
			w.WriteMsg(m)
			return
		}
	}

	// ── Resolve client group membership ─────────────────────────────────────
	var groupScopeKey string
	var groupBlocked int
	var groupRules string
	if clientIP != "unknown" {
		var gid string
		_ = s.db.QueryRow(`
			SELECT g.id FROM client_groups g
			JOIN client_group_members m ON m.group_id = g.id
			WHERE m.identifier = ? LIMIT 1`, clientIP,
		).Scan(&gid)
		if gid != "" {
			groupScopeKey = "group:" + gid
			_ = s.db.QueryRow(`SELECT blocked, rules FROM client_groups WHERE id = ?`, gid).
				Scan(&groupBlocked, &groupRules)
		}
	}

	scopeForCache := groupScopeKey
	if scopeForCache == "" {
		scopeForCache = clientIP
	}
	cacheKey := qname + ":" + strconv.Itoa(int(qtype)) + ":" + scopeForCache

	// ── DNS cache lookup ─────────────────────────────────────────────────────
	if entry, ok := s.dnsCache.get(cacheKey); ok && time.Now().Before(entry.expiresAt) {
		w.WriteMsg(entry.msg)
		return
	}

	// ── Predefined service blocks ────────────────────────────────────────────
	serviceScopeKey := clientIP
	if groupScopeKey != "" {
		serviceScopeKey = groupScopeKey
	}
	if s.checkServiceBlock(qname, serviceScopeKey) {
		s.logQueryReason(qname, qtype, clientIP, true, "service-block", 1.0, "service-block")
		reply := s.blackholeReply(req, 60)
		s.dnsCache.set(cacheKey, dnsCacheEntry{msg: reply.Copy(), expiresAt: time.Now().Add(dnsCacheTTL)})
		w.WriteMsg(reply)
		return
	}

	// ── Per-client / per-group rules ─────────────────────────────────────────
	// skipGlobalBlock is set when an allow rule matches.
	skipGlobalBlock := false
	if clientIP != "unknown" {
		var clientBlocked int
		var clientRules string
		_ = s.db.QueryRow("SELECT blocked, rules FROM client_rules WHERE client_ip = ?", clientIP).
			Scan(&clientBlocked, &clientRules)

		if clientRules != "" {
			if action, matched := parseClientRules(clientRules, qname); matched {
				if action == "block" {
					s.logQueryReason(qname, qtype, clientIP, true, "client-block", 1.0, "client-block")
					reply := s.blackholeReply(req, 300)
					s.dnsCache.set(cacheKey, dnsCacheEntry{msg: reply.Copy(), expiresAt: time.Now().Add(dnsCacheTTL)})
					w.WriteMsg(reply)
					return
				}
				s.logQueryReason(qname, qtype, clientIP, false, "client-allow", 1.0, "client-allow")
				skipGlobalBlock = true
			}
		}

		if !skipGlobalBlock && clientBlocked == 1 {
			s.logQueryReason(qname, qtype, clientIP, true, "client-blocked", 1.0, "client-blocked")
			reply := s.blackholeReply(req, 300)
			s.dnsCache.set(cacheKey, dnsCacheEntry{msg: reply.Copy(), expiresAt: time.Now().Add(dnsCacheTTL)})
			w.WriteMsg(reply)
			return
		}

		if !skipGlobalBlock && groupRules != "" {
			if action, matched := parseClientRules(groupRules, qname); matched {
				if action == "block" {
					s.logQueryReason(qname, qtype, clientIP, true, "group-block", 1.0, "group-block")
					reply := s.blackholeReply(req, 300)
					s.dnsCache.set(cacheKey, dnsCacheEntry{msg: reply.Copy(), expiresAt: time.Now().Add(dnsCacheTTL)})
					w.WriteMsg(reply)
					return
				}
				s.logQueryReason(qname, qtype, clientIP, false, "group-allow", 1.0, "group-allow")
				skipGlobalBlock = true
			}
		}

		if !skipGlobalBlock && groupBlocked == 1 {
			s.logQueryReason(qname, qtype, clientIP, true, "group-blocked", 1.0, "group-blocked")
			reply := s.blackholeReply(req, 300)
			s.dnsCache.set(cacheKey, dnsCacheEntry{msg: reply.Copy(), expiresAt: time.Now().Add(dnsCacheTTL)})
			w.WriteMsg(reply)
			return
		}
	}

	// ── Global blocklist ─────────────────────────────────────────────────────
	if !skipGlobalBlock && s.blocklistEnabled() && s.blocklistHas(qname) {
		s.logQueryReason(qname, qtype, clientIP, true, "blocklist", 1.0, "blocklist")
		reply := s.blackholeReply(req, 300)
		s.dnsCache.set(cacheKey, dnsCacheEntry{msg: reply.Copy(), expiresAt: time.Now().Add(dnsCacheTTL)})
		w.WriteMsg(reply)
		return
	}

	// ── ML classification ────────────────────────────────────────────────────
	if !skipGlobalBlock && s.mlEnabled() && (qtype == dns.TypeA || qtype == dns.TypeAAAA) && !isMLAllowlisted(qname) {
		isMal, cat, conf, mlErr := s.classifyDomain(qname)
		if mlErr == nil && s.applyMLCategoryFilter(isMal, cat, conf) {
			s.logQueryReason(qname, qtype, clientIP, true, cat, conf, "ml:"+strings.ToLower(cat))
			reply := s.blackholeReply(req, 300)
			s.dnsCache.set(cacheKey, dnsCacheEntry{msg: reply.Copy(), expiresAt: time.Now().Add(dnsCacheTTL)})
			w.WriteMsg(reply)
			return
		}
		if mlErr == nil {
			s.logQueryReason(qname, qtype, clientIP, false, cat, conf, "allowed")
		} else {
			s.logQueryReason(qname, qtype, clientIP, false, "unknown", 0, "allowed")
		}
	} else if qtype == dns.TypeA || qtype == dns.TypeAAAA {
		s.logQueryReason(qname, qtype, clientIP, false, "n/a", 0, "allowed")
	}

	// ── Proxy to upstreams ───────────────────────────────────────────────────
	in, err := s.queryUpstreams(req)
	if err != nil {
		s.log(LogLevelWarn, "dns", map[string]any{"action": "upstream_lookup", "error": err.Error()})
		m := new(dns.Msg)
		m.SetRcode(req, dns.RcodeServerFailure)
		w.WriteMsg(m)
		return
	}
	s.dnsCache.set(cacheKey, dnsCacheEntry{msg: in.Copy(), expiresAt: time.Now().Add(dnsCacheTTL)})
	w.WriteMsg(in)
}

// logQueryReason inserts a DNS query record using the pre-prepared statement.
func (s *GuardianServer) logQueryReason(domain string, qtype uint16, clientIP string, blocked bool, category string, confidence float32, reason string) {
	s.dbMu.Lock()
	defer s.dbMu.Unlock()
	_, err := s.logStmt.Exec(
		time.Now().UTC().Format(time.RFC3339),
		domain, int(qtype), clientIP,
		boolToInt(blocked), category, float64(confidence),
		reason,
	)
	if err != nil {
		s.log(LogLevelError, "db", map[string]any{"action": "insert_query", "error": err.Error()})
	}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ── Upstream DNS ──────────────────────────────────────────────────────────────

func (s *GuardianServer) queryUpstreams(req *dns.Msg) (*dns.Msg, error) {
	s.upstreamMu.RLock()
	upstreams := make([]string, len(s.upstreams))
	copy(upstreams, s.upstreams)
	s.upstreamMu.RUnlock()

	if len(upstreams) == 0 {
		return nil, fmt.Errorf("no upstream DNS servers configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	type result struct {
		msg *dns.Msg
		err error
	}
	resCh := make(chan result, len(upstreams))
	var wg sync.WaitGroup
	for _, addr := range upstreams {
		addr := addr
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := singleUpstreamLookup(ctx, req.Copy(), addr)
			resCh <- result{msg: resp, err: err}
		}()
	}
	go func() {
		wg.Wait()
		close(resCh)
	}()

	var lastErr error
	for {
		select {
		case <-ctx.Done():
			if lastErr == nil {
				lastErr = ctx.Err()
			}
			return nil, lastErr
		case res, ok := <-resCh:
			if !ok {
				if lastErr == nil {
					lastErr = fmt.Errorf("no upstream responses")
				}
				return nil, lastErr
			}
			if res.err == nil && res.msg != nil {
				return res.msg, nil
			}
			if res.err != nil {
				lastErr = res.err
			}
		}
	}
}

func singleUpstreamLookup(ctx context.Context, req *dns.Msg, addr string) (*dns.Msg, error) {
	client := &dns.Client{Timeout: 2 * time.Second, Net: "udp"}
	if resp, _, err := client.ExchangeContext(ctx, req, addr); err == nil {
		return resp, nil
	}
	client.Net = "tcp"
	resp, _, err := client.ExchangeContext(ctx, req, addr)
	return resp, err
}

func parseUpstreamServers(raw string) []string {
	replacer := strings.NewReplacer(",", " ", ";", " ", "\n", " ", "\r", " ")
	var upstreams []string
	for _, field := range strings.Fields(replacer.Replace(raw)) {
		addr := strings.TrimSpace(field)
		if addr == "" {
			continue
		}
		if !strings.Contains(addr, ":") {
			addr += ":53"
		}
		upstreams = append(upstreams, addr)
	}
	return upstreams
}

// ── Auth / session ────────────────────────────────────────────────────────────

func (s *GuardianServer) authenticateUser(username, password string) (int64, error) {
	var id int64
	var hash []byte
	if err := s.db.QueryRow("SELECT id, password_hash FROM users WHERE username = ?", username).Scan(&id, &hash); err != nil {
		return 0, fmt.Errorf("invalid credentials")
	}
	if bcrypt.CompareHashAndPassword(hash, []byte(password)) != nil {
		return 0, fmt.Errorf("invalid credentials")
	}
	return id, nil
}

func (s *GuardianServer) createSession(w http.ResponseWriter, userID int64) (string, error) {
	token := randomHex(32)
	expires := time.Now().Add(sessionTTL)
	if _, err := s.db.Exec("INSERT INTO sessions (token, user_id, expires_at) VALUES (?, ?, ?)",
		token, userID, expires.Format(time.RFC3339)); err != nil {
		return "", err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	return token, nil
}

func (s *GuardianServer) getUserFromSession(r *http.Request) (string, error) {
	c, err := r.Cookie(sessionCookieName)
	if err != nil {
		return "", fmt.Errorf("no session")
	}
	var uid int64
	var expires string
	if err := s.db.QueryRow("SELECT user_id, expires_at FROM sessions WHERE token = ?", c.Value).
		Scan(&uid, &expires); err != nil {
		return "", fmt.Errorf("invalid session")
	}
	t, _ := time.Parse(time.RFC3339, expires)
	if time.Now().After(t) {
		return "", fmt.Errorf("session expired")
	}
	var username string
	if err := s.db.QueryRow("SELECT username FROM users WHERE id = ?", uid).Scan(&username); err != nil {
		return "", fmt.Errorf("user not found")
	}
	return username, nil
}

// ── HTTP middleware ───────────────────────────────────────────────────────────

// withAuth wraps a handler with optional CORS and session authentication.
// If method is non-empty, only that HTTP method is accepted.
func (s *GuardianServer) withAuth(
	frontendDev bool,
	method string,
	fn func(w http.ResponseWriter, r *http.Request, user string),
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, frontendDev)
		if frontendDev && r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		if method != "" && r.Method != method {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		user, err := s.getUserFromSession(r)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		fn(w, r, user)
	}
}

// withAuthAny is like withAuth but accepts any HTTP method.
func (s *GuardianServer) withAuthAny(
	frontendDev bool,
	fn func(w http.ResponseWriter, r *http.Request, user string),
) http.HandlerFunc {
	return s.withAuth(frontendDev, "", fn)
}

// withNoAuth wraps a handler with optional CORS but does NOT require a session.
func withNoAuth(
	frontendDev bool,
	method string,
	fn func(w http.ResponseWriter, r *http.Request),
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, frontendDev)
		if frontendDev && r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		if method != "" && r.Method != method {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		fn(w, r)
	}
}

// applyCORS sets CORS headers when running in frontend-dev mode.
func applyCORS(w http.ResponseWriter, _ *http.Request, frontendDev bool) {
	if !frontendDev {
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "http://localhost:5173")
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

// securityHeaders adds defensive HTTP response headers to every response.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'")
		next.ServeHTTP(w, r)
	})
}

// ── HTTP helpers ──────────────────────────────────────────────────────────────

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func jsonErr(w http.ResponseWriter, msg string, code int) {
	http.Error(w, msg, code)
}

// ── HTTP handler registration ─────────────────────────────────────────────────

func (s *GuardianServer) startServers(listen, webAddr string, frontendDev, oneExe bool) error {
	dns.HandleFunc(".", s.handleDNSQuery)
	udp := &dns.Server{Addr: listen, Net: "udp"}
	tcp := &dns.Server{Addr: listen, Net: "tcp"}
	go func() {
		log.Printf("[dns] starting UDP server on %s", listen)
		if err := udp.ListenAndServe(); err != nil {
			log.Fatalf("[dns] udp listen failed: %v", err)
		}
	}()
	go func() {
		log.Printf("[dns] starting TCP server on %s", listen)
		if err := tcp.ListenAndServe(); err != nil {
			log.Fatalf("[dns] tcp listen failed: %v", err)
		}
	}()

	s.registerAuthHandlers(frontendDev)
	s.registerStatsHandlers(frontendDev)
	s.registerQueryHandlers(frontendDev)
	s.registerBlocklistHandlers(frontendDev)
	s.registerServiceHandlers(frontendDev)
	s.registerClientHandlers(frontendDev)
	s.registerGroupHandlers(frontendDev)
	s.registerUpstreamHandlers(frontendDev)
	s.registerMLHandlers(frontendDev)
	s.registerSettingsHandlers(frontendDev)
	s.registerSPAHandler(oneExe)

	wrappedMux := securityHeaders(http.DefaultServeMux)

	if frontendDev {
		log.Printf("[web] frontend dev mode enabled; CORS allowed for http://localhost:5173")
	} else {
		log.Printf("[web] serving SPA at %s", webAddr)
	}
	log.Printf("[web] starting on %s", webAddr)
	go func() {
		if err := http.ListenAndServe(webAddr, wrappedMux); err != nil {
			log.Fatalf("[web] http listen failed: %v", err)
		}
	}()
	return nil
}

// ── Auth handlers ─────────────────────────────────────────────────────────────

func (s *GuardianServer) registerAuthHandlers(dev bool) {
	http.HandleFunc("/api/login", withNoAuth(dev, http.MethodPost, func(w http.ResponseWriter, r *http.Request) {
		var cred struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&cred); err != nil {
			jsonErr(w, "bad request", http.StatusBadRequest)
			return
		}
		uid, err := s.authenticateUser(cred.Username, cred.Password)
		if err != nil {
			jsonErr(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if _, err := s.createSession(w, uid); err != nil {
			jsonErr(w, "server error", http.StatusInternalServerError)
			return
		}
		jsonOK(w, map[string]any{"ok": true})
	}))

	http.HandleFunc("/api/logout", withNoAuth(dev, http.MethodPost, func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie(sessionCookieName); err == nil {
			_, _ = s.db.Exec("DELETE FROM sessions WHERE token = ?", c.Value)
			http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: "", Path: "/", HttpOnly: true, MaxAge: -1})
		}
		jsonOK(w, map[string]any{"ok": true})
	}))

	http.HandleFunc("/api/user", s.withAuth(dev, http.MethodGet, func(w http.ResponseWriter, r *http.Request, user string) {
		jsonOK(w, map[string]any{"username": user})
	}))

	http.HandleFunc("/api/change-password", s.withAuth(dev, http.MethodPost, func(w http.ResponseWriter, r *http.Request, user string) {
		var body struct {
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Password == "" {
			jsonErr(w, "password cannot be empty", http.StatusBadRequest)
			return
		}
		hash, _ := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
		if _, err := s.db.Exec("UPDATE users SET password_hash = ? WHERE username = ?", hash, user); err != nil {
			jsonErr(w, "failed to update password", http.StatusInternalServerError)
			return
		}
		jsonOK(w, map[string]any{"ok": true})
	}))

	http.HandleFunc("/api/setup-needed", withNoAuth(dev, http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		var count int
		if err := s.db.QueryRow("SELECT COUNT(1) FROM users").Scan(&count); err != nil {
			jsonErr(w, "db error", http.StatusInternalServerError)
			return
		}
		needed := count == 0
		if count == 1 {
			var username string
			var hash []byte
			if err := s.db.QueryRow("SELECT username, password_hash FROM users LIMIT 1").Scan(&username, &hash); err == nil && username == "admin" {
				needed = bcrypt.CompareHashAndPassword(hash, []byte("admin")) == nil
			}
		}
		jsonOK(w, map[string]any{"needed": needed})
	}))

	http.HandleFunc("/api/setup", withNoAuth(dev, http.MethodPost, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Username == "" || body.Password == "" {
			jsonErr(w, "username and password required", http.StatusBadRequest)
			return
		}
		var count int
		_ = s.db.QueryRow("SELECT COUNT(1) FROM users").Scan(&count)
		if count > 0 {
			jsonErr(w, "setup already completed", http.StatusConflict)
			return
		}
		hash, _ := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
		if _, err := s.db.Exec("INSERT INTO users (username, password_hash) VALUES (?, ?)", body.Username, hash); err != nil {
			jsonErr(w, "failed to create user", http.StatusInternalServerError)
			return
		}
		jsonOK(w, map[string]any{"ok": true})
	}))

	http.HandleFunc("/api/version", withNoAuth(dev, http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		jsonOK(w, map[string]any{"version": AppVersion})
	}))

	http.HandleFunc("/api/current-ip", withNoAuth(dev, http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		ip := "127.0.0.1"
		if addrs, err := net.InterfaceAddrs(); err == nil {
			for _, addr := range addrs {
				if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
					ip = ipnet.IP.String()
					break
				}
			}
		}
		jsonOK(w, map[string]any{"ip": ip})
	}))
}

// ── Stats handler ─────────────────────────────────────────────────────────────

func (s *GuardianServer) registerStatsHandlers(dev bool) {
	http.HandleFunc("/api/stats", s.withAuth(dev, http.MethodGet, func(w http.ResponseWriter, r *http.Request, _ string) {
		var total, blocked, mlBlocked int
		_ = s.db.QueryRow("SELECT COUNT(1) FROM queries").Scan(&total)
		_ = s.db.QueryRow("SELECT COUNT(1) FROM queries WHERE blocked=1").Scan(&blocked)
		_ = s.db.QueryRow("SELECT COUNT(1) FROM queries WHERE blocked=1 AND category != 'blocklist'").Scan(&mlBlocked)

		since24h := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
		var total24h, blocked24h int
		_ = s.db.QueryRow("SELECT COUNT(1) FROM queries WHERE timestamp >= ?", since24h).Scan(&total24h)
		_ = s.db.QueryRow("SELECT COUNT(1) FROM queries WHERE blocked=1 AND timestamp >= ?", since24h).Scan(&blocked24h)

		topDomains := s.queryTopRows(
			`SELECT domain, COUNT(1) FROM queries GROUP BY domain ORDER BY COUNT(1) DESC LIMIT 10`,
			"domain",
		)
		topBlocked := s.queryTopRows(
			`SELECT domain, COUNT(1) FROM queries WHERE blocked=1 GROUP BY domain ORDER BY COUNT(1) DESC LIMIT 5`,
			"domain",
		)

		qtypeRows, _ := s.db.Query(`SELECT qtype, COUNT(1) FROM queries GROUP BY qtype ORDER BY COUNT(1) DESC`)
		qtypeBreakdown := []map[string]any{}
		if qtypeRows != nil {
			defer qtypeRows.Close()
			for qtypeRows.Next() {
				var qt, cnt int
				if qtypeRows.Scan(&qt, &cnt) == nil {
					qtypeBreakdown = append(qtypeBreakdown, map[string]any{
						"qtype": qt, "label": qtypeToString(uint16(qt)), "count": cnt,
					})
				}
			}
		}

		catRows, _ := s.db.Query(`SELECT category, COUNT(1) FROM queries WHERE blocked=1 GROUP BY category ORDER BY COUNT(1) DESC`)
		catBreakdown := []map[string]any{}
		if catRows != nil {
			defer catRows.Close()
			for catRows.Next() {
				var cat string
				var cnt int
				if catRows.Scan(&cat, &cnt) == nil {
					catBreakdown = append(catBreakdown, map[string]any{"category": cat, "count": cnt})
				}
			}
		}

		s.mlConnMu.RLock()
		mlConnected := s.mlCli != nil
		s.mlConnMu.RUnlock()

		jsonOK(w, map[string]any{
			"total":           total,
			"blocked":         blocked,
			"ml_blocked":      mlBlocked,
			"total_24h":       total24h,
			"blocked_24h":     blocked24h,
			"top_domains":     topDomains,
			"top_blocked":     topBlocked,
			"qtype_breakdown": qtypeBreakdown,
			"cat_breakdown":   catBreakdown,
			"ml_enabled":      s.mlEnabled(),
			"ml_connected":    mlConnected,
		})
	}))
}

// queryTopRows runs a "SELECT label, COUNT(*)" query and returns the result as
// a slice of {"domain"/"category": label, "count": n} maps.
func (s *GuardianServer) queryTopRows(query, labelKey string) []map[string]any {
	rows, err := s.db.Query(query)
	out := []map[string]any{}
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var label string
		var cnt int
		if rows.Scan(&label, &cnt) == nil {
			out = append(out, map[string]any{labelKey: label, "count": cnt})
		}
	}
	return out
}

// ── Query log handlers ────────────────────────────────────────────────────────

func (s *GuardianServer) registerQueryHandlers(dev bool) {
	http.HandleFunc("/api/queries", s.withAuth(dev, http.MethodGet, s.handleGetQueries))
	http.HandleFunc("/api/queries/allow", s.withAuth(dev, http.MethodPost, s.handleQuickAllow))
	http.HandleFunc("/api/queries/block", s.withAuth(dev, http.MethodPost, s.handleQuickBlock))
	http.HandleFunc("/api/queries/retention", s.withAuthAny(dev, s.handleQueryRetention))
	http.HandleFunc("/api/dns/test", s.withAuth(dev, http.MethodGet, s.handleDNSTest))
	http.HandleFunc("/api/test-domain", s.withAuth(dev, http.MethodGet, s.handleDNSTest))
	http.HandleFunc("/queries/export", s.withAuth(dev, http.MethodGet, s.handleQueryExport))
}

func (s *GuardianServer) handleGetQueries(w http.ResponseWriter, r *http.Request, _ string) {
	q := r.URL.Query().Get("q")
	client := r.URL.Query().Get("client")
	blockedOnly := r.URL.Query().Get("blocked") == "1"
	qtypeFilter := r.URL.Query().Get("type")

	limit := 50
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v <= 500 {
		limit = v
	}
	offset := 0
	if v, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil && v >= 0 {
		offset = v
	}

	// Use correlated scalar subqueries instead of LEFT JOINs to prevent
	// fan-out duplicates when a client belongs to multiple groups.
	const baseSelect = `
		SELECT q.timestamp, q.domain, q.qtype, q.client_ip, q.blocked,
		       q.category, q.confidence,
		       COALESCE(
		           (SELECT cg.name
		            FROM client_group_members cgm
		            JOIN client_groups cg ON cg.id = cgm.group_id
		            WHERE cgm.identifier = q.client_ip LIMIT 1),
		           (SELECT cr.label FROM client_rules cr WHERE cr.client_ip = q.client_ip LIMIT 1),
		           ''
		       ) AS client_label,
		       COALESCE(q.reason, q.category, '') AS reason
		FROM queries q`
	const baseCount = `SELECT COUNT(*) FROM queries q`

	where := " WHERE 1=1"
	args := []any{}

	if client != "" {
		where += " AND q.client_ip LIKE ?"
		args = append(args, "%"+client+"%")
	} else if q != "" {
		where += ` AND (q.domain LIKE ? OR q.client_ip LIKE ?
			OR EXISTS (
				SELECT 1 FROM client_group_members cgm
				JOIN client_groups cg ON cg.id = cgm.group_id
				WHERE cgm.identifier = q.client_ip AND cg.name LIKE ?
			)
			OR EXISTS (
				SELECT 1 FROM client_rules cr
				WHERE cr.client_ip = q.client_ip AND cr.label LIKE ?
			))`
		args = append(args, "%"+q+"%", "%"+q+"%", "%"+q+"%", "%"+q+"%")
	}
	if blockedOnly {
		where += " AND q.blocked = 1"
	}
	if qtypeFilter != "" {
		qtypeMap := map[string]int{
			"A": 1, "NS": 2, "CNAME": 5, "SOA": 6,
			"PTR": 12, "MX": 15, "TXT": 16, "AAAA": 28,
			"SRV": 33, "ANY": 255,
		}
		if qt, ok := qtypeMap[strings.ToUpper(qtypeFilter)]; ok {
			where += " AND q.qtype = ?"
			args = append(args, qt)
		}
	}

	var total int
	countArgs := make([]any, len(args))
	copy(countArgs, args)
	_ = s.db.QueryRow(baseCount+where, countArgs...).Scan(&total)

	rows, err := s.db.Query(
		baseSelect+where+" ORDER BY q.timestamp DESC LIMIT ? OFFSET ?",
		append(args, limit, offset)...,
	)
	if err != nil {
		jsonErr(w, "db error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	out := []map[string]any{}
	for rows.Next() {
		var ts, dom, clientIP, cat, label, reason string
		var qtype, blocked int
		var conf float64
		_ = rows.Scan(&ts, &dom, &qtype, &clientIP, &blocked, &cat, &conf, &label, &reason)
		out = append(out, map[string]any{
			"timestamp":    ts,
			"domain":       dom,
			"qtype":        qtype,
			"client_ip":    clientIP,
			"blocked":      blocked == 1,
			"category":     cat,
			"confidence":   conf,
			"client_label": label,
			"reason":       reason,
		})
	}
	w.Header().Set("X-Total-Count", strconv.Itoa(total))
	jsonOK(w, out)
}

func (s *GuardianServer) handleQuickAllow(w http.ResponseWriter, r *http.Request, _ string) {
	var body struct {
		Domain string `json:"domain"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Domain) == "" {
		jsonErr(w, "bad request: domain required", http.StatusBadRequest)
		return
	}
	domain := strings.ToLower(strings.TrimSpace(body.Domain))
	allowRule := "@@||" + domain + "^"

	s.dbMu.Lock()
	var existing string
	_ = s.db.QueryRow("SELECT rules FROM custom_rules LIMIT 1").Scan(&existing)
	alreadyPresent := false
	for _, l := range strings.Split(strings.TrimSpace(existing), "\n") {
		if strings.TrimSpace(l) == allowRule {
			alreadyPresent = true
			break
		}
	}
	if !alreadyPresent {
		updated := strings.TrimSpace(existing)
		if updated != "" {
			updated += "\n"
		}
		updated += allowRule
		if existing == "" {
			_, _ = s.db.Exec("INSERT OR IGNORE INTO custom_rules (rules) VALUES (?)", updated)
		}
		_, _ = s.db.Exec("UPDATE custom_rules SET rules = ?", updated)
	}
	s.dbMu.Unlock()

	if !alreadyPresent {
		s.blocklistRemove(domain)
		s.evictDNSByPrefix(domain + ":")
		s.mlCache.evictExact(domain)
		s.log(LogLevelInfo, "allow", map[string]any{"action": "quick_allow", "domain": domain})
	}

	jsonOK(w, map[string]any{"ok": true, "domain": domain, "already_present": alreadyPresent})
}

func (s *GuardianServer) handleQuickBlock(w http.ResponseWriter, r *http.Request, _ string) {
	var body struct {
		Domain string `json:"domain"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Domain) == "" {
		jsonErr(w, "bad request: domain required", http.StatusBadRequest)
		return
	}
	domain := strings.ToLower(strings.TrimSpace(body.Domain))
	s.blocklistAdd(domain)
	s.evictDNSByPrefix(domain + ":")
	s.log(LogLevelInfo, "block", map[string]any{"action": "quick_block", "domain": domain})
	jsonOK(w, map[string]any{"ok": true, "domain": domain})
}

func (s *GuardianServer) handleQueryRetention(w http.ResponseWriter, r *http.Request, _ string) {
	switch r.Method {
	case http.MethodGet:
		var val string
		_ = s.db.QueryRow("SELECT value FROM settings WHERE key='queries_retain_days'").Scan(&val)
		days := 90
		if v, err := strconv.Atoi(val); err == nil && v > 0 {
			days = v
		}
		jsonOK(w, map[string]any{"days": days})
	case http.MethodPost:
		var body struct {
			Days int `json:"days"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Days < 1 {
			jsonErr(w, "bad request: days must be >= 1", http.StatusBadRequest)
			return
		}
		cutoff := time.Now().UTC().AddDate(0, 0, -body.Days).Format(time.RFC3339)
		s.dbMu.Lock()
		_, _ = s.db.Exec("INSERT OR REPLACE INTO settings (key, value) VALUES ('queries_retain_days', ?)", strconv.Itoa(body.Days))
		_, _ = s.db.Exec("DELETE FROM queries WHERE timestamp < ?", cutoff)
		s.dbMu.Unlock()
		s.log(LogLevelInfo, "db", map[string]any{"action": "retention_updated", "days": body.Days})
		jsonOK(w, map[string]any{"ok": true, "days": body.Days})
	default:
		jsonErr(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *GuardianServer) handleDNSTest(w http.ResponseWriter, r *http.Request, _ string) {
	domain := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("domain")))
	if domain == "" {
		jsonErr(w, "domain required", http.StatusBadRequest)
		return
	}
	clientIP := strings.TrimSpace(r.URL.Query().Get("client"))
	if clientIP == "" {
		clientIP = strings.TrimSpace(r.URL.Query().Get("client_ip"))
	}

	type testResult struct {
		Domain     string   `json:"domain"`
		ClientIP   string   `json:"client_ip"`
		Blocked    bool     `json:"blocked"`
		Reason     string   `json:"reason"`
		Category   string   `json:"category"`
		Confidence float64  `json:"confidence"`
		Checks     []string `json:"checks"`
	}
	res := testResult{Domain: domain, ClientIP: clientIP, Reason: "allowed"}
	checks := &res.Checks

	finishBlocked := func(reason string) {
		res.Blocked = true
		res.Reason = reason
		jsonOK(w, res)
	}

	// 1. Resolve group scope key
	serviceScopeKey := clientIP
	if clientIP != "" {
		var gid string
		_ = s.db.QueryRow(`
			SELECT g.id FROM client_groups g
			JOIN client_group_members m ON m.group_id = g.id
			WHERE m.identifier = ? LIMIT 1`, clientIP,
		).Scan(&gid)
		if gid != "" {
			serviceScopeKey = "group:" + gid
		}
	}

	// 2. Service block
	if s.checkServiceBlock(domain, serviceScopeKey) {
		*checks = append(*checks, "service-block: matched")
		finishBlocked("service-block")
		return
	}
	*checks = append(*checks, "service-block: no match")

	// 3. Per-client / per-group rules
	skipGlobal := false
	if clientIP != "" {
		var clientBlocked int
		var clientRules string
		_ = s.db.QueryRow("SELECT blocked, rules FROM client_rules WHERE client_ip = ?", clientIP).
			Scan(&clientBlocked, &clientRules)

		if clientRules != "" {
			action, matched := parseClientRules(clientRules, domain)
			if matched {
				if action == "block" {
					*checks = append(*checks, "client-rule: block matched")
					finishBlocked("client-block")
					return
				}
				*checks = append(*checks, "client-rule: allow matched — bypasses global blocklist")
				skipGlobal = true
			} else {
				*checks = append(*checks, "client-rule: no match")
			}
		}
		if !skipGlobal && clientBlocked == 1 {
			*checks = append(*checks, "client globally blocked")
			finishBlocked("client-blocked")
			return
		}

		if !skipGlobal && serviceScopeKey != clientIP {
			gidStr := strings.TrimPrefix(serviceScopeKey, "group:")
			var groupBlocked int
			var groupRules string
			_ = s.db.QueryRow(`SELECT blocked, rules FROM client_groups WHERE id = ?`, gidStr).
				Scan(&groupBlocked, &groupRules)
			if groupRules != "" {
				action, matched := parseClientRules(groupRules, domain)
				if matched {
					if action == "block" {
						*checks = append(*checks, "group-rule: block matched")
						finishBlocked("group-block")
						return
					}
					*checks = append(*checks, "group-rule: allow matched — bypasses global blocklist")
					skipGlobal = true
				} else {
					*checks = append(*checks, "group-rule: no match")
				}
			}
			if !skipGlobal && groupBlocked == 1 {
				*checks = append(*checks, "group globally blocked")
				finishBlocked("group-blocked")
				return
			}
		}
	}

	// 4. Global blocklist
	if !skipGlobal && s.blocklistEnabled() {
		if s.blocklistHas(domain) {
			*checks = append(*checks, "blocklist: matched")
			res.Category = "blocklist"
			finishBlocked("blocklist")
			return
		}
		*checks = append(*checks, "blocklist: no match")
	} else if skipGlobal {
		*checks = append(*checks, "blocklist: skipped (client allow rule)")
	} else {
		*checks = append(*checks, "blocklist: disabled")
	}

	// 5. ML classification
	if !skipGlobal && s.mlEnabled() && !isMLAllowlisted(domain) {
		isMal, cat, conf, mlErr := s.classifyDomain(domain)
		res.Category = cat
		res.Confidence = float64(conf)
		ml := s.mlSettingsAtomic.Load()
		if mlErr != nil {
			*checks = append(*checks, fmt.Sprintf("ml: error (%v)", mlErr))
		} else if s.applyMLCategoryFilter(isMal, cat, conf) {
			*checks = append(*checks, fmt.Sprintf("ml: blocked (cat=%s conf=%.2f thresh=%.2f)", cat, conf, ml.threshold))
			finishBlocked("ml:" + strings.ToLower(cat))
			return
		} else {
			*checks = append(*checks, fmt.Sprintf("ml: allowed (cat=%s conf=%.2f thresh=%.2f)", cat, conf, ml.threshold))
		}
	} else if skipGlobal {
		*checks = append(*checks, "ml: skipped (client allow rule)")
	} else if !s.mlEnabled() {
		*checks = append(*checks, "ml: disabled")
	} else {
		*checks = append(*checks, "ml: skipped (allowlisted)")
	}

	jsonOK(w, res)
}

func (s *GuardianServer) handleQueryExport(w http.ResponseWriter, r *http.Request, _ string) {
	limit := 10000
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v < 500000 {
		limit = v
	}
	rows, err := s.db.Query(
		`SELECT timestamp, domain, qtype, client_ip, blocked, category, confidence,
		        COALESCE(reason, category, '') AS reason
		 FROM queries ORDER BY timestamp DESC LIMIT ?`, limit,
	)
	if err != nil {
		jsonErr(w, "db error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	filename := fmt.Sprintf("queries_%s.csv", time.Now().Format("20060102-150405"))
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"timestamp", "domain", "qtype", "client_ip", "blocked", "category", "confidence", "reason"})
	for rows.Next() {
		var ts, dom, client, cat, reason string
		var qtype, blocked int
		var conf float64
		_ = rows.Scan(&ts, &dom, &qtype, &client, &blocked, &cat, &conf, &reason)
		_ = cw.Write([]string{ts, dom, qtypeToString(uint16(qtype)), client, strconv.Itoa(blocked), cat, fmt.Sprintf("%.4f", conf), reason})
	}
	cw.Flush()
}

// ── Blocklist handlers ────────────────────────────────────────────────────────

func (s *GuardianServer) registerBlocklistHandlers(dev bool) {
	http.HandleFunc("/api/blocklist", s.withAuthAny(dev, s.handleBlocklist))
	http.HandleFunc("/api/blocklist/predefined", s.withAuth(dev, http.MethodGet, func(w http.ResponseWriter, r *http.Request, _ string) {
		jsonOK(w, predefinedBlocklists)
	}))
	http.HandleFunc("/api/blocklist/load", s.withAuth(dev, http.MethodPost, s.handleBlocklistLoad))
	http.HandleFunc("/api/blocklist/sources", s.withAuthAny(dev, s.handleBlocklistSources))
	http.HandleFunc("/api/blocklist/reload", s.withAuth(dev, http.MethodPost, func(w http.ResponseWriter, r *http.Request, _ string) {
		go s.reloadAllSources()
		s.log(LogLevelInfo, "blocklist", map[string]any{"action": "manual_reload"})
		jsonOK(w, map[string]any{"ok": true})
	}))
	http.HandleFunc("/api/blocklist/clear", s.withAuth(dev, http.MethodPost, func(w http.ResponseWriter, r *http.Request, _ string) {
		s.dbMu.Lock()
		_, _ = s.db.Exec("DELETE FROM blocklist_sources")
		_, _ = s.db.Exec("DELETE FROM custom_rules")
		s.dbMu.Unlock()
		s.blMu.Lock()
		s.blocklist = make(map[string]struct{})
		s.blMu.Unlock()
		_ = s.persistBlocklistToFile()
		s.log(LogLevelInfo, "blocklist", map[string]any{"action": "cleared"})
		jsonOK(w, map[string]any{"ok": true})
	}))
	http.HandleFunc("/api/blocklist/toggle", s.withAuthAny(dev, s.handleBlocklistToggle))
	http.HandleFunc("/api/blocklist/rules", s.withAuthAny(dev, s.handleBlocklistRules))
}

func (s *GuardianServer) handleBlocklist(w http.ResponseWriter, r *http.Request, _ string) {
	switch r.Method {
	case http.MethodGet:
		s.blMu.RLock()
		list := make([]string, 0, len(s.blocklist))
		for d := range s.blocklist {
			list = append(list, d)
		}
		s.blMu.RUnlock()
		sort.Strings(list)
		jsonOK(w, list)
	case http.MethodPost:
		var body struct {
			Domain string `json:"domain"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			jsonErr(w, "bad request", http.StatusBadRequest)
			return
		}
		s.blocklistAdd(body.Domain)
		w.WriteHeader(http.StatusNoContent)
	case http.MethodDelete:
		d := r.URL.Query().Get("domain")
		if d == "" {
			var body struct {
				Domain string `json:"domain"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
				d = body.Domain
			}
		}
		if d != "" {
			s.blocklistRemove(d)
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		jsonErr(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *GuardianServer) handleBlocklistLoad(w http.ResponseWriter, r *http.Request, _ string) {
	var body struct {
		URLs    []string `json:"urls"`
		Sources []struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"sources"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonErr(w, "bad request", http.StatusBadRequest)
		return
	}
	if len(body.URLs) == 0 {
		for _, ns := range body.Sources {
			body.URLs = append(body.URLs, ns.URL)
		}
	}
	s.dbMu.Lock()
	for _, url := range body.URLs {
		name := url
		for _, ns := range body.Sources {
			if ns.URL == url {
				name = ns.Name
				break
			}
		}
		_, _ = s.db.Exec("INSERT OR IGNORE INTO blocklist_sources (name, url) VALUES (?, ?)", name, url)
	}
	s.dbMu.Unlock()
	go s.reloadAllSources()
	jsonOK(w, map[string]any{"ok": true})
}

func (s *GuardianServer) handleBlocklistSources(w http.ResponseWriter, r *http.Request, _ string) {
	switch r.Method {
	case http.MethodGet:
		rows, err := s.db.Query("SELECT id, name, url FROM blocklist_sources ORDER BY id")
		if err != nil {
			jsonErr(w, "db error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		out := []map[string]any{}
		for rows.Next() {
			var id int
			var name, url string
			if rows.Scan(&id, &name, &url) == nil {
				out = append(out, map[string]any{"id": id, "name": name, "url": url})
			}
		}
		jsonOK(w, out)
	case http.MethodDelete:
		var body struct {
			URL string `json:"url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.URL == "" {
			jsonErr(w, "bad request", http.StatusBadRequest)
			return
		}
		s.dbMu.Lock()
		_, err := s.db.Exec("DELETE FROM blocklist_sources WHERE url = ?", body.URL)
		s.dbMu.Unlock()
		if err != nil {
			jsonErr(w, "db error", http.StatusInternalServerError)
			return
		}
		go s.reloadAllSources()
		jsonOK(w, map[string]any{"ok": true})
	default:
		jsonErr(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *GuardianServer) handleBlocklistToggle(w http.ResponseWriter, r *http.Request, _ string) {
	switch r.Method {
	case http.MethodGet:
		jsonOK(w, map[string]any{"enabled": s.blocklistEnabled()})
	case http.MethodPost:
		var body struct {
			Enabled bool `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			jsonErr(w, "bad request", http.StatusBadRequest)
			return
		}
		v := int32(0)
		if body.Enabled {
			v = 1
		}
		atomic.StoreInt32(&s.blocklistEnabledAtomic, v)
		s.saveToggleToDB("blocklist_enabled", body.Enabled)
		s.log(LogLevelInfo, "blocklist", map[string]any{"action": "toggle", "enabled": body.Enabled})
		jsonOK(w, map[string]any{"ok": true, "enabled": body.Enabled})
	default:
		jsonErr(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *GuardianServer) handleBlocklistRules(w http.ResponseWriter, r *http.Request, _ string) {
	switch r.Method {
	case http.MethodGet:
		var rules string
		_ = s.db.QueryRow("SELECT rules FROM custom_rules ORDER BY id DESC LIMIT 1").Scan(&rules)
		jsonOK(w, map[string]any{"rules": rules})
	case http.MethodPost:
		var body struct {
			Rules string `json:"rules"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			jsonErr(w, "bad request", http.StatusBadRequest)
			return
		}
		s.dbMu.Lock()
		_, _ = s.db.Exec("DELETE FROM custom_rules")
		_, _ = s.db.Exec("INSERT INTO custom_rules (rules) VALUES (?)", body.Rules)
		s.dbMu.Unlock()
		go s.reloadAllSources()
		jsonOK(w, map[string]any{"ok": true})
	default:
		jsonErr(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// ── Service handlers ──────────────────────────────────────────────────────────

func (s *GuardianServer) registerServiceHandlers(dev bool) {
	http.HandleFunc("/api/services/definitions", s.withAuth(dev, http.MethodGet, func(w http.ResponseWriter, r *http.Request, _ string) {
		jsonOK(w, PredefinedServices)
	}))
	http.HandleFunc("/api/services", s.withAuthAny(dev, s.handleServices))
}

func (s *GuardianServer) handleServices(w http.ResponseWriter, r *http.Request, _ string) {
	switch r.Method {
	case http.MethodGet:
		scope := r.URL.Query().Get("scope")
		if scope == "" {
			scope = "global"
		}
		scopeKey := r.URL.Query().Get("key")

		// merged=1 returns the effective schedule for each service: client row
		// if present, otherwise global row, with a "source" field for the UI.
		if r.URL.Query().Get("merged") == "1" && scope == "client" && scopeKey != "" {
			clientRows := s.fetchScheduleRows("client", scopeKey)
			globalRows := s.fetchScheduleRows("global", "")
			out := map[string]map[string]any{}
			for id, row := range globalRows {
				out[id] = row
			}
			for id, row := range clientRows {
				out[id] = row // client always wins
			}
			jsonOK(w, out)
			return
		}

		rows, err := s.db.Query(
			`SELECT service_id, enabled, days_of_week, time_start, time_end
				   FROM service_schedules WHERE scope=? AND scope_key=?`,
			scope, scopeKey,
		)
		if err != nil {
			jsonErr(w, "db error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		out := map[string]map[string]any{}
		for rows.Next() {
			var svcID, daysCSV, tStart, tEnd string
			var enabled int
			if rows.Scan(&svcID, &enabled, &daysCSV, &tStart, &tEnd) == nil {
				out[svcID] = map[string]any{
					"enabled":      enabled == 1,
					"days_of_week": daysCSV,
					"time_start":   tStart,
					"time_end":     tEnd,
				}
			}
		}
		jsonOK(w, out)

	case http.MethodPost:
		var body struct {
			Scope      string `json:"scope"`
			ScopeKey   string `json:"scope_key"`
			ServiceID  string `json:"service_id"`
			Enabled    bool   `json:"enabled"`
			DaysOfWeek string `json:"days_of_week"`
			TimeStart  string `json:"time_start"`
			TimeEnd    string `json:"time_end"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ServiceID == "" {
			jsonErr(w, "bad request", http.StatusBadRequest)
			return
		}
		if body.Scope == "" {
			body.Scope = "global"
		}
		enabledInt := 0
		if body.Enabled {
			enabledInt = 1
		}
		s.dbMu.Lock()
		_, err := s.db.Exec(
			`INSERT INTO service_schedules (scope, scope_key, service_id, enabled, days_of_week, time_start, time_end)
				 VALUES (?, ?, ?, ?, ?, ?, ?)
				 ON CONFLICT(scope, scope_key, service_id) DO UPDATE SET
				   enabled=excluded.enabled,
				   days_of_week=excluded.days_of_week,
				   time_start=excluded.time_start,
				   time_end=excluded.time_end`,
			body.Scope, body.ScopeKey, body.ServiceID, enabledInt, body.DaysOfWeek, body.TimeStart, body.TimeEnd,
		)
		s.dbMu.Unlock()
		if err != nil {
			jsonErr(w, "db error", http.StatusInternalServerError)
			return
		}
		s.flushServiceSchedCache(body.ServiceID)
		s.flushServiceDNSCache(body.ServiceID)
		jsonOK(w, map[string]any{"ok": true})

	case http.MethodDelete:
		var body struct {
			Scope     string `json:"scope"`
			ScopeKey  string `json:"scope_key"`
			ServiceID string `json:"service_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ServiceID == "" || body.Scope == "" || body.ScopeKey == "" {
			jsonErr(w, "bad request: scope, scope_key and service_id required", http.StatusBadRequest)
			return
		}
		s.dbMu.Lock()
		_, err := s.db.Exec(
			`DELETE FROM service_schedules WHERE scope=? AND scope_key=? AND service_id=?`,
			body.Scope, body.ScopeKey, body.ServiceID,
		)
		s.dbMu.Unlock()
		if err != nil {
			jsonErr(w, "db error", http.StatusInternalServerError)
			return
		}
		s.flushServiceSchedCache(body.ServiceID)
		s.flushServiceDNSCache(body.ServiceID)
		jsonOK(w, map[string]any{"ok": true})

	default:
		jsonErr(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// fetchScheduleRows returns a map of service_id -> schedule row for the given scope.
func (s *GuardianServer) fetchScheduleRows(scope, scopeKey string) map[string]map[string]any {
	out := map[string]map[string]any{}
	rows, err := s.db.Query(
		`SELECT service_id, enabled, days_of_week, time_start, time_end
			   FROM service_schedules WHERE scope=? AND scope_key=?`,
		scope, scopeKey,
	)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var svcID, daysCSV, tStart, tEnd string
		var enabled int
		if rows.Scan(&svcID, &enabled, &daysCSV, &tStart, &tEnd) == nil {
			out[svcID] = map[string]any{
				"enabled":      enabled == 1,
				"days_of_week": daysCSV,
				"time_start":   tStart,
				"time_end":     tEnd,
				"source":       scope,
			}
		}
	}
	return out
}

// ── Client handlers ───────────────────────────────────────────────────────────

func (s *GuardianServer) registerClientHandlers(dev bool) {
	http.HandleFunc("/api/clients", s.withAuthAny(dev, s.handleClients))
}

func (s *GuardianServer) handleClients(w http.ResponseWriter, r *http.Request, _ string) {
	switch r.Method {
	case http.MethodGet:
		rows, err := s.db.Query(
			"SELECT id, client_ip, label, blocked, rules, created_at FROM client_rules ORDER BY id",
		)
		if err != nil {
			jsonErr(w, "db error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		out := []map[string]any{}
		for rows.Next() {
			var id, blocked int
			var ip, label, rules, createdAt string
			if rows.Scan(&id, &ip, &label, &blocked, &rules, &createdAt) == nil {
				out = append(out, map[string]any{
					"id":         id,
					"client_ip":  ip,
					"label":      label,
					"blocked":    blocked == 1,
					"rules":      rules,
					"created_at": createdAt,
				})
			}
		}
		jsonOK(w, out)

	case http.MethodPost:
		var body struct {
			ClientIP string `json:"client_ip"`
			Label    string `json:"label"`
			Blocked  bool   `json:"blocked"`
			Rules    string `json:"rules"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ClientIP == "" {
			jsonErr(w, "bad request", http.StatusBadRequest)
			return
		}
		blockedInt := 0
		if body.Blocked {
			blockedInt = 1
		}
		s.dbMu.Lock()
		_, err := s.db.Exec(
			`INSERT INTO client_rules (client_ip, label, blocked, rules)
				 VALUES (?, ?, ?, ?)
				 ON CONFLICT(client_ip) DO UPDATE SET
				   label   = excluded.label,
				   blocked = excluded.blocked,
				   rules   = excluded.rules`,
			body.ClientIP, body.Label, blockedInt, body.Rules,
		)
		s.dbMu.Unlock()
		if err != nil {
			jsonErr(w, "db error", http.StatusInternalServerError)
			return
		}
		s.log(LogLevelInfo, "clients", map[string]any{"action": "upsert", "client": body.ClientIP})
		jsonOK(w, map[string]any{"ok": true})

	case http.MethodDelete:
		var body struct {
			ClientIP string `json:"client_ip"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ClientIP == "" {
			jsonErr(w, "bad request", http.StatusBadRequest)
			return
		}
		s.dbMu.Lock()
		_, err := s.db.Exec("DELETE FROM client_rules WHERE client_ip = ?", body.ClientIP)
		s.dbMu.Unlock()
		if err != nil {
			jsonErr(w, "db error", http.StatusInternalServerError)
			return
		}
		s.log(LogLevelInfo, "clients", map[string]any{"action": "delete", "client": body.ClientIP})
		jsonOK(w, map[string]any{"ok": true})

	default:
		jsonErr(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// ── Group handlers ────────────────────────────────────────────────────────────

func (s *GuardianServer) registerGroupHandlers(dev bool) {
	http.HandleFunc("/api/groups", s.withAuthAny(dev, s.handleGroups))
	http.HandleFunc("/api/groups/members", s.withAuthAny(dev, s.handleGroupMembers))
}

func (s *GuardianServer) handleGroups(w http.ResponseWriter, r *http.Request, _ string) {
	switch r.Method {
	case http.MethodGet:
		rows, err := s.db.Query(
			`SELECT id, name, label, blocked, rules, created_at FROM client_groups ORDER BY id`,
		)
		if err != nil {
			jsonErr(w, "db error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		type groupRow struct {
			ID        int              `json:"id"`
			Name      string           `json:"name"`
			Label     string           `json:"label"`
			Blocked   bool             `json:"blocked"`
			Rules     string           `json:"rules"`
			CreatedAt string           `json:"created_at"`
			Members   []map[string]any `json:"members"`
		}
		var groups []groupRow
		for rows.Next() {
			var g groupRow
			var blocked int
			if rows.Scan(&g.ID, &g.Name, &g.Label, &blocked, &g.Rules, &g.CreatedAt) == nil {
				g.Blocked = blocked == 1
				g.Members = []map[string]any{}
				groups = append(groups, g)
			}
		}
		rows.Close()
		for i, g := range groups {
			mrows, merr := s.db.Query(
				`SELECT id, identifier, type FROM client_group_members WHERE group_id = ? ORDER BY id`,
				g.ID,
			)
			if merr == nil {
				for mrows.Next() {
					var mid int
					var ident, mtype string
					if mrows.Scan(&mid, &ident, &mtype) == nil {
						groups[i].Members = append(groups[i].Members, map[string]any{
							"id":         mid,
							"identifier": ident,
							"type":       mtype,
						})
					}
				}
				mrows.Close()
			}
		}
		if groups == nil {
			groups = []groupRow{}
		}
		jsonOK(w, groups)

	case http.MethodPost:
		var body struct {
			ID      int    `json:"id"`
			Name    string `json:"name"`
			Label   string `json:"label"`
			Blocked bool   `json:"blocked"`
			Rules   string `json:"rules"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
			jsonErr(w, "bad request", http.StatusBadRequest)
			return
		}
		blockedInt := 0
		if body.Blocked {
			blockedInt = 1
		}
		s.dbMu.Lock()
		var newID int64
		var dbErr error
		if body.ID == 0 {
			var res sql.Result
			res, dbErr = s.db.Exec(
				`INSERT INTO client_groups (name, label, blocked, rules) VALUES (?, ?, ?, ?)`,
				body.Name, body.Label, blockedInt, body.Rules,
			)
			if dbErr == nil {
				newID, _ = res.LastInsertId()
			}
		} else {
			_, dbErr = s.db.Exec(
				`UPDATE client_groups SET name=?, label=?, blocked=?, rules=? WHERE id=?`,
				body.Name, body.Label, blockedInt, body.Rules, body.ID,
			)
			newID = int64(body.ID)
		}
		s.dbMu.Unlock()
		if dbErr != nil {
			jsonErr(w, "db error", http.StatusInternalServerError)
			return
		}
		s.log(LogLevelInfo, "groups", map[string]any{"action": "upsert", "id": newID, "name": body.Name})
		jsonOK(w, map[string]any{"ok": true, "id": newID})

	case http.MethodDelete:
		var body struct {
			ID int `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == 0 {
			jsonErr(w, "bad request", http.StatusBadRequest)
			return
		}
		s.dbMu.Lock()
		_, err := s.db.Exec(`DELETE FROM client_groups WHERE id = ?`, body.ID)
		s.dbMu.Unlock()
		if err != nil {
			jsonErr(w, "db error", http.StatusInternalServerError)
			return
		}
		s.log(LogLevelInfo, "groups", map[string]any{"action": "delete", "id": body.ID})
		jsonOK(w, map[string]any{"ok": true})

	default:
		jsonErr(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *GuardianServer) handleGroupMembers(w http.ResponseWriter, r *http.Request, _ string) {
	switch r.Method {
	case http.MethodPost:
		var body struct {
			GroupID    int    `json:"group_id"`
			Identifier string `json:"identifier"`
			Type       string `json:"type"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.GroupID == 0 || body.Identifier == "" {
			jsonErr(w, "bad request", http.StatusBadRequest)
			return
		}
		if body.Type == "" {
			body.Type = "ip"
		}
		s.dbMu.Lock()
		_, err := s.db.Exec(
			`INSERT OR IGNORE INTO client_group_members (group_id, identifier, type) VALUES (?, ?, ?)`,
			body.GroupID, strings.ToLower(strings.TrimSpace(body.Identifier)), body.Type,
		)
		s.dbMu.Unlock()
		if err != nil {
			jsonErr(w, "db error", http.StatusInternalServerError)
			return
		}
		jsonOK(w, map[string]any{"ok": true})

	case http.MethodDelete:
		var body struct {
			GroupID    int    `json:"group_id"`
			Identifier string `json:"identifier"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.GroupID == 0 || body.Identifier == "" {
			jsonErr(w, "bad request", http.StatusBadRequest)
			return
		}
		s.dbMu.Lock()
		_, err := s.db.Exec(
			`DELETE FROM client_group_members WHERE group_id = ? AND identifier = ?`,
			body.GroupID, strings.ToLower(strings.TrimSpace(body.Identifier)),
		)
		s.dbMu.Unlock()
		if err != nil {
			jsonErr(w, "db error", http.StatusInternalServerError)
			return
		}
		jsonOK(w, map[string]any{"ok": true})

	default:
		jsonErr(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// ── Upstream handlers ─────────────────────────────────────────────────────────

func (s *GuardianServer) registerUpstreamHandlers(dev bool) {
	http.HandleFunc("/api/upstream", s.withAuthAny(dev, func(w http.ResponseWriter, r *http.Request, _ string) {
		switch r.Method {
		case http.MethodGet:
			s.upstreamMu.RLock()
			servers := strings.Join(s.upstreams, "\n")
			s.upstreamMu.RUnlock()
			jsonOK(w, map[string]any{"servers": servers})
		case http.MethodPost:
			var body struct {
				Servers string `json:"servers"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				jsonErr(w, "bad request", http.StatusBadRequest)
				return
			}
			newUpstreams := parseUpstreamServers(body.Servers)
			if len(newUpstreams) == 0 {
				jsonErr(w, "at least one upstream server required", http.StatusBadRequest)
				return
			}
			s.upstreamMu.Lock()
			s.upstreams = newUpstreams
			s.upstreamMu.Unlock()
			s.saveUpstreamsToDB(newUpstreams)
			s.log(LogLevelInfo, "upstream", map[string]any{"action": "updated", "count": len(newUpstreams)})
			jsonOK(w, map[string]any{"ok": true})
		default:
			jsonErr(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))
}

// ── ML handlers ───────────────────────────────────────────────────────────────

func (s *GuardianServer) registerMLHandlers(dev bool) {
	http.HandleFunc("/api/ml/toggle", s.withAuthAny(dev, func(w http.ResponseWriter, r *http.Request, _ string) {
		switch r.Method {
		case http.MethodGet:
			jsonOK(w, map[string]any{"enabled": s.mlEnabled()})
		case http.MethodPost:
			var body struct {
				Enabled bool `json:"enabled"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				jsonErr(w, "bad request", http.StatusBadRequest)
				return
			}
			v := int32(0)
			if body.Enabled {
				v = 1
			}
			atomic.StoreInt32(&s.mlEnabledAtomic, v)
			s.saveToggleToDB("ml_enabled", body.Enabled)
			s.log(LogLevelInfo, "ml", map[string]any{"action": "toggle", "enabled": body.Enabled})
			jsonOK(w, map[string]any{"ok": true, "enabled": body.Enabled})
		default:
			jsonErr(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	http.HandleFunc("/api/ml/settings", s.withAuthAny(dev, s.handleMLSettings))
	http.HandleFunc("/api/ml/feedback", s.withAuth(dev, http.MethodPost, s.handleMLFeedback))
	http.HandleFunc("/api/ml/feedback/export", s.withAuth(dev, http.MethodGet, s.handleMLFeedbackExport))
}

func (s *GuardianServer) handleMLSettings(w http.ResponseWriter, r *http.Request, _ string) {
	switch r.Method {
	case http.MethodGet:
		ml := s.mlSettingsAtomic.Load()
		s.mlCache.mu.Lock()
		cacheSize := s.mlCache.order.Len()
		s.mlCache.mu.Unlock()
		var totalFeedback, safeFeedback, malFeedback int
		_ = s.db.QueryRow("SELECT COUNT(*) FROM ml_feedback").Scan(&totalFeedback)
		_ = s.db.QueryRow("SELECT COUNT(*) FROM ml_feedback WHERE verdict='safe'").Scan(&safeFeedback)
		_ = s.db.QueryRow("SELECT COUNT(*) FROM ml_feedback WHERE verdict='malicious'").Scan(&malFeedback)
		s.mlConnMu.RLock()
		mlConnected := s.mlCli != nil
		s.mlConnMu.RUnlock()
		jsonOK(w, map[string]any{
			"threshold":      ml.threshold,
			"block_dga":      ml.blockDGA,
			"block_phishing": ml.blockPhishing,
			"block_malware":  ml.blockMalware,
			"block_other":    ml.blockOther,
			"cache_size":     cacheSize,
			"cache_max":      maxMLCacheSize,
			"cache_ttl_min":  int(mlCacheTTL.Minutes()),
			"feedback_total": totalFeedback,
			"feedback_safe":  safeFeedback,
			"feedback_mal":   malFeedback,
			"ml_connected":   mlConnected,
		})

	case http.MethodPost:
		var body struct {
			Threshold     *float32 `json:"threshold"`
			BlockDGA      *bool    `json:"block_dga"`
			BlockPhishing *bool    `json:"block_phishing"`
			BlockMalware  *bool    `json:"block_malware"`
			BlockOther    *bool    `json:"block_other"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			jsonErr(w, "bad request", http.StatusBadRequest)
			return
		}
		cur := s.mlSettingsAtomic.Load()
		next := *cur
		if body.Threshold != nil {
			t := *body.Threshold
			if t < 0.1 {
				t = 0.1
			}
			if t > 1.0 {
				t = 1.0
			}
			next.threshold = t
			s.dbMu.Lock()
			_, _ = s.db.Exec(
				"INSERT INTO settings (key,value) VALUES ('ml_threshold',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value",
				strconv.FormatFloat(float64(t), 'f', 4, 32),
			)
			s.dbMu.Unlock()
		}
		if body.BlockDGA != nil {
			next.blockDGA = *body.BlockDGA
			s.saveToggleToDB("ml_block_dga", *body.BlockDGA)
		}
		if body.BlockPhishing != nil {
			next.blockPhishing = *body.BlockPhishing
			s.saveToggleToDB("ml_block_phishing", *body.BlockPhishing)
		}
		if body.BlockMalware != nil {
			next.blockMalware = *body.BlockMalware
			s.saveToggleToDB("ml_block_malware", *body.BlockMalware)
		}
		if body.BlockOther != nil {
			next.blockOther = *body.BlockOther
			s.saveToggleToDB("ml_block_other", *body.BlockOther)
		}
		s.mlSettingsAtomic.Store(&next)
		// Clear ML cache so new settings take effect immediately.
		s.mlCache.mu.Lock()
		s.mlCache.items = make(map[string]*list.Element, s.mlCache.cap)
		s.mlCache.order.Init()
		s.mlCache.mu.Unlock()
		s.log(LogLevelInfo, "ml", map[string]any{"action": "settings_updated"})
		jsonOK(w, map[string]any{"ok": true})

	default:
		jsonErr(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *GuardianServer) handleMLFeedback(w http.ResponseWriter, r *http.Request, _ string) {
	var body struct {
		Domain     string  `json:"domain"`
		Verdict    string  `json:"verdict"`
		Category   string  `json:"category"`
		Confidence float64 `json:"confidence"`
		ClientIP   string  `json:"client_ip"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Domain == "" {
		jsonErr(w, "bad request", http.StatusBadRequest)
		return
	}
	if body.Verdict != "safe" && body.Verdict != "malicious" {
		jsonErr(w, "verdict must be 'safe' or 'malicious'", http.StatusBadRequest)
		return
	}
	s.dbMu.Lock()
	_, err := s.db.Exec(
		"INSERT INTO ml_feedback (domain, verdict, category, confidence, client_ip) VALUES (?, ?, ?, ?, ?)",
		body.Domain, body.Verdict, body.Category, body.Confidence, body.ClientIP,
	)
	s.dbMu.Unlock()
	if err != nil {
		jsonErr(w, "db error", http.StatusInternalServerError)
		return
	}
	if body.Verdict == "safe" {
		s.mlCache.evictExact(strings.ToLower(strings.TrimSpace(body.Domain)))
	}
	s.log(LogLevelInfo, "ml", map[string]any{"action": "feedback", "domain": body.Domain, "verdict": body.Verdict})
	jsonOK(w, map[string]any{"ok": true})
}

func (s *GuardianServer) handleMLFeedbackExport(w http.ResponseWriter, r *http.Request, _ string) {
	rows, err := s.db.Query(
		"SELECT domain, verdict, category, confidence, client_ip, created_at FROM ml_feedback ORDER BY created_at DESC",
	)
	if err != nil {
		jsonErr(w, "db error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	fname := fmt.Sprintf("ml_feedback_%s.csv", time.Now().Format("20060102-150405"))
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", fname))
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"domain", "verdict", "category", "confidence", "client_ip", "created_at"})
	for rows.Next() {
		var domain, verdict, category, clientIP, createdAt string
		var confidence float64
		_ = rows.Scan(&domain, &verdict, &category, &confidence, &clientIP, &createdAt)
		_ = cw.Write([]string{domain, verdict, category, fmt.Sprintf("%.4f", confidence), clientIP, createdAt})
	}
	cw.Flush()
}

// ── Settings / misc handlers ──────────────────────────────────────────────────

func (s *GuardianServer) registerSettingsHandlers(_ bool) {
	// All settings-related endpoints are already registered via the specific
	// handler groups above. This hook exists for future additions.
}

// ── SPA handler ───────────────────────────────────────────────────────────────

func (s *GuardianServer) registerSPAHandler(oneExe bool) {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api") {
			http.NotFound(w, r)
			return
		}
		var fs http.FileSystem
		if oneExe {
			sub, err := iofs.Sub(embeddedDist, "frontend/dist")
			if err != nil {
				fs = http.Dir("./frontend/dist")
			} else {
				fs = http.FS(sub)
			}
		} else {
			fs = http.Dir("./frontend/dist")
		}

		path := r.URL.Path
		if !strings.Contains(path, ".") {
			path = "/index.html"
		}

		f, err := fs.Open(path)
		if err != nil {
			if path != "/index.html" {
				f, err = fs.Open("/index.html")
				if err != nil {
					http.NotFound(w, r)
					return
				}
				path = "/index.html"
			} else {
				http.NotFound(w, r)
				return
			}
		}
		defer f.Close()

		stat, err := f.Stat()
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		http.ServeContent(w, r, path, stat.ModTime(), f)
	})
}

// ── Logging ───────────────────────────────────────────────────────────────────

func (s *GuardianServer) log(level LogLevel, component string, fields map[string]any) {
	if level > s.logLevel {
		return
	}
	entry := map[string]any{
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"level":     logLevelNames[level],
		"component": component,
	}
	for k, v := range fields {
		entry[k] = v
	}
	if b, err := json.Marshal(entry); err == nil {
		log.Println(string(b))
	} else {
		log.Printf("[%s] %s: %v", component, logLevelNames[level], fields)
	}
}

// ── Embedded file helpers ─────────────────────────────────────────────────────

// writeEmbeddedDir extracts an embedded FS subdirectory to a destination path,
// preserving relative structure. Makes .py/.sh files executable.
func writeEmbeddedDir(efs embed.FS, src, dest string) error {
	return iofs.WalkDir(efs, src, func(path string, d iofs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		out := filepath.Join(dest, rel)
		if d.IsDir() {
			if rel == "." {
				return nil
			}
			return os.MkdirAll(out, 0o755)
		}
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		inf, err := efs.Open(path)
		if err != nil {
			return err
		}
		defer inf.Close()
		outf, err := os.Create(out)
		if err != nil {
			return err
		}
		defer outf.Close()
		if _, err := io.Copy(outf, inf); err != nil {
			return err
		}
		lower := strings.ToLower(path)
		if strings.HasSuffix(lower, ".py") || strings.HasSuffix(lower, ".sh") {
			_ = os.Chmod(out, 0o755)
		}
		return nil
	})
}

// ── Python ML subprocess ──────────────────────────────────────────────────────

func isTermux() bool {
	// Termux sets PREFIX to /data/data/com.termux/files/usr (or similar).
	prefix := os.Getenv("PREFIX")
	if strings.Contains(prefix, "com.termux") {
		return true
	}
	// Fallback: check if the Termux files directory exists.
	if _, err := os.Stat("/data/data/com.termux"); err == nil {
		return true
	}
	return false
}

func startEmbeddedPython(mlDir string) (*exec.Cmd, error) {
	python := "python"
	if _, err := exec.LookPath(python); err != nil {
		python = "python3"
		if _, err2 := exec.LookPath(python); err2 != nil {
			return nil, fmt.Errorf("python executable not found in PATH")
		}
	}

	termux := isTermux()
	if termux {
		log.Printf("[ml] Termux environment detected")
	}

	// Only install Python dependencies if a key module is missing.
	reqPath := filepath.Join(mlDir, "requirements.txt")
	if _, err := os.Stat(reqPath); err == nil {
		check := exec.Command(python, "-c", "import grpc, numpy, onnxruntime")
		check.Dir = mlDir
		if err := check.Run(); err != nil {
			log.Printf("[ml] missing Python dependencies, installing ...")

			if termux {
				// On Termux, grpcio and onnxruntime often lack prebuilt wheels.
				// Try the Termux system package manager first for native packages,
				// then fall back to pip with --break-system-packages for the rest.
				log.Printf("[ml] attempting Termux pkg install for native packages ...")
				for _, pkg := range []string{"python-numpy", "python-grpcio", "python-onnxruntime"} {
					pkgCmd := exec.Command("pkg", "install", "-y", pkg)
					pkgCmd.Dir = mlDir
					pkgCmd.Stdout = os.Stdout
					pkgCmd.Stderr = os.Stderr
					if err := pkgCmd.Run(); err != nil {
						log.Printf("[ml] pkg install %s not available, will try pip", pkg)
					}
				}
				// Re-check after pkg install; only pip-install what's still missing.
				recheck := exec.Command(python, "-c", "import grpc, numpy, onnxruntime")
				recheck.Dir = mlDir
				if err := recheck.Run(); err != nil {
					log.Printf("[ml] still missing deps after pkg, falling back to pip ...")
					pip := exec.Command(python, "-m", "pip", "install",
						"--quiet", "--disable-pip-version-check",
						"--break-system-packages",
						"-r", reqPath)
					pip.Dir = mlDir
					pip.Stdout = os.Stdout
					pip.Stderr = os.Stderr
					if err := pip.Run(); err != nil {
						log.Printf("[ml] warning: pip install failed: %v", err)
						log.Printf("[ml] On Termux, install dependencies manually:")
						log.Printf("[ml]   pkg install python-numpy python-grpcio python-onnxruntime")
						log.Printf("[ml]   pip install typing_extensions")
					}
				}
			} else {
				pip := exec.Command(python, "-m", "pip", "install", "--quiet", "--disable-pip-version-check", "-r", reqPath)
				pip.Dir = mlDir
				pip.Stdout = os.Stdout
				pip.Stderr = os.Stderr
				if err := pip.Run(); err != nil {
					log.Printf("[ml] warning: pip install failed: %v (ML service may not start)", err)
				}
			}
		} else {
			log.Printf("[ml] Python dependencies already installed, skipping pip install")
		}
	}

	cmd := exec.Command(python, "guardian_grpc.py")
	cmd.Dir = mlDir
	cmd.Env = append(os.Environ(),
		"PYTHONWARNINGS=ignore",
		"GRPC_VERBOSITY=ERROR",
	)
	var bufOut, bufErr bytes.Buffer
	cmd.Stdout = io.MultiWriter(os.Stdout, &bufOut)
	cmd.Stderr = io.MultiWriter(os.Stderr, &bufErr)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	go func() {
		_ = cmd.Wait()
		log.Printf("[ml] embedded python exited; stdout:\n%s\nstderr:\n%s\n", bufOut.String(), bufErr.String())
	}()
	return cmd, nil
}

// ── Misc utilities ────────────────────────────────────────────────────────────

func qtypeToString(t uint16) string {
	switch t {
	case dns.TypeA:
		return "A"
	case dns.TypeAAAA:
		return "AAAA"
	case dns.TypeCNAME:
		return "CNAME"
	case dns.TypeMX:
		return "MX"
	case dns.TypeTXT:
		return "TXT"
	default:
		return strconv.Itoa(int(t))
	}
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ── Entry point ───────────────────────────────────────────────────────────────

func main() {
	listen := flag.String("listen", ":53", "DNS listen address (udp/tcp)")
	upstream := flag.String("upstream", "8.8.8.8:53 1.1.1.1:53", "Upstream DNS server(s), space-separated")
	blockfile := flag.String("blocklist", defaultBlockfile, "Blocklist hosts-style file")
	mlAddr := flag.String("ml", "localhost:50051", "ML gRPC address")
	dbPath := flag.String("db", defaultDBPath, "SQLite DB path")
	webAddr := flag.String("web", ":8081", "Web UI listen address")
	logLevelStr := flag.String("log-level", "warn", "Log level (error, warn, info, debug)")
	verbose := flag.Bool("verbose", false, "Enable info-level logging (shorthand for --log-level info)")
	frontendDev := flag.Bool("frontend-dev", false, "Enable CORS for Vite dev server at http://localhost:5173")
	_ = flag.Bool("one-exe", true, "Deprecated: always-on (kept for backward compatibility)")
	flag.Parse()

	if *verbose && *logLevelStr == "warn" {
		*logLevelStr = "info"
	}

	if isTermux() && *listen == ":53" {
		log.Printf("[guardian] WARNING: Termux detected — port 53 requires root.")
		log.Printf("[guardian] If bind fails, restart with: --listen :5353")
		log.Printf("[guardian] Then point your device DNS to <this-ip>:5353")
	}

	var logLevel LogLevel
	switch strings.ToLower(*logLevelStr) {
	case "error":
		logLevel = LogLevelError
	case "warn":
		logLevel = LogLevelWarn
	case "info":
		logLevel = LogLevelInfo
	case "debug":
		logLevel = LogLevelDebug
	default:
		logLevel = LogLevelInfo
	}

	// Extract and start the embedded ML service.
	td, err := os.MkdirTemp("", "guardian-ai-ml-*")
	if err != nil {
		log.Fatalf("failed to create temp dir for embedded ML: %v", err)
	}
	if err := writeEmbeddedDir(embeddedML, "ml-service", td); err != nil {
		log.Fatalf("failed to extract embedded ML service: %v", err)
	}
	log.Printf("[ml] extracted embedded ML service to %s", td)

	// The model/tokenizer may already have been extracted by writeEmbeddedDir
	// (if they were included in the go:embed directive). If not, try to copy
	// them from common disk locations next to the executable.
	exeDir := filepath.Dir(func() string { p, _ := os.Executable(); return p }())
	for _, modelFile := range []string{"tokenizer.pickle"} {
		dest := filepath.Join(td, modelFile)
		if _, err := os.Stat(dest); err == nil {
			log.Printf("[ml] %s found (embedded)", modelFile)
			continue
		}
		candidates := []string{
			filepath.Join(exeDir, modelFile),
			filepath.Join(exeDir, "ml-service", modelFile),
			filepath.Join("ml-service", modelFile),
			modelFile,
		}
		copied := false
		for _, src := range candidates {
			data, err := os.ReadFile(src)
			if err != nil {
				continue
			}
			if err := os.WriteFile(dest, data, 0o644); err != nil {
				log.Printf("[ml] warning: found %s at %s but could not copy: %v", modelFile, src, err)
			} else {
				log.Printf("[ml] copied %s from %s", modelFile, src)
				copied = true
			}
			break
		}
		if !copied {
			log.Printf("[ml] warning: %s not found — ML will fail until model is trained (run ml-service/train_model.py)", modelFile)
		}
	}

	mlCmd, err := startEmbeddedPython(td)
	if err != nil {
		log.Fatalf("failed to start embedded python ML: %v", err)
	}

	// srv is declared before the signal handler so the closure can reference it.
	var srv *GuardianServer

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		log.Printf("[guardian] shutting down")
		if mlCmd != nil && mlCmd.Process != nil {
			_ = mlCmd.Process.Signal(syscall.SIGTERM)
		}
		if srv != nil {
			srv.saveDNSCacheToDB()
		}
		os.Exit(0)
	}()

	srv, err = NewGuardianServer(*upstream, *mlAddr, *blockfile, *dbPath, logLevel)
	if err != nil {
		log.Fatalf("failed to create server: %v", err)
	}

	if err := srv.startServers(*listen, *webAddr, *frontendDev, true); err != nil {
		log.Fatalf("failed to start servers: %v", err)
	}

	select {} // block forever
}
