package config

import (
	"sort"
	"strings"
	"time"
)

// ── Constants ─────────────────────────────────────────────────────────────────

const (
	DefaultDBPath       = "guardian.db"
	DefaultBlockfile    = "blocklists/hosts.txt"
	SessionCookieName   = "guardian_session"
	SessionTTL          = 24 * time.Hour
	MLCacheTTL          = 5 * time.Minute
	MaxMLCacheSize      = 10_000
	DNSCacheTTL         = 60 * time.Second
	MaxDNSCacheSize     = 50_000
	RateLimitPerMin     = 1000
	RateLimitBurst      = 200 // token-bucket burst capacity per client
	SvcScheduleCacheTTL = 10 * time.Second
)

// ── Predefined blocklist sources ──────────────────────────────────────────────

// PredefinedBlocklists is the static list of well-known blocklist sources
// offered in the Settings UI. Defined at package level so it is not
// re-allocated on every HTTP request.
var PredefinedBlocklists = []map[string]string{
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

// ServiceDomainIndex maps domain -> []service_id for fast lookup in the DNS path.
var ServiceDomainIndex map[string][]string

// MLAllowlist holds domains that should never be sent to the ML model.
// These are well-known legitimate domains; the model produces false positives on
// many of them (e.g. roblox.com scores as "Phishing" due to training-data bias).
var MLAllowlist map[string]struct{}

// ExtraMLAllowlist is the static set of platform/CDN/infrastructure domains
// added to MLAllowlist at init time in addition to all predefined service domains.
var ExtraMLAllowlist = []string{
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

// MLAllowlistSuffixes is a sorted slice of ".domain" strings built from
// MLAllowlist at init time, used for O(log n) subdomain matching.
var MLAllowlistSuffixes []string

func init() {
	// Build service domain index and seed ML allowlist from all predefined service domains.
	ServiceDomainIndex = make(map[string][]string)
	MLAllowlist = make(map[string]struct{})

	for _, svc := range PredefinedServices {
		for _, d := range svc.Domains {
			d = strings.ToLower(d)
			ServiceDomainIndex[d] = append(ServiceDomainIndex[d], svc.ID)
			MLAllowlist[d] = struct{}{}
		}
	}

	for _, d := range ExtraMLAllowlist {
		MLAllowlist[strings.ToLower(d)] = struct{}{}
	}

	// Build sorted suffix slice for binary-search subdomain lookup.
	MLAllowlistSuffixes = make([]string, 0, len(MLAllowlist))
	for d := range MLAllowlist {
		MLAllowlistSuffixes = append(MLAllowlistSuffixes, "."+d)
	}
	sort.Strings(MLAllowlistSuffixes)
}

// IsMLAllowlisted checks if a domain is in the ML allowlist (exact match or subdomain match).
func IsMLAllowlisted(domain string) bool {
	domain = strings.ToLower(domain)

	// Keep popping off subdomains until we find a match or run out
	parts := strings.Split(domain, ".")
	for i := 0; i < len(parts); i++ {
		check := strings.Join(parts[i:], ".")
		if _, ok := MLAllowlist[check]; ok {
			return true
		}
	}
	return false
}
