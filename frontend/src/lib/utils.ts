import type { ReasonColors, DNSInstructionSet } from "./types";

export function relativeTime(iso: string): string {
  const diff = Math.floor((Date.now() - new Date(iso).getTime()) / 1000);
  if (diff < 5) return "just now";
  if (diff < 60) return `${diff}s ago`;
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`;
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`;
  return `${Math.floor(diff / 86400)}d ago`;
}

export function formatTime(iso: string): string {
  try {
    return new Date(iso).toLocaleTimeString([], {
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
    });
  } catch {
    return iso;
  }
}

/**
 * Returns background/foreground/border colors for a block-reason badge.
 * Used consistently across Dashboard, Query Log, and the Block Reasons tile.
 */
export function reasonBadgeColor(
  cat: string,
  reason?: string,
): ReasonColors | null {
  const c = cat.toLowerCase();
  const r = (reason ?? "").toLowerCase();
  const isServiceBlock =
    c === "service-block" ||
    r === "service-block" ||
    c === "service" ||
    r === "service" ||
    c.startsWith("service:") ||
    r.startsWith("service:");
  const isClientBlock =
    c === "client-block" ||
    c === "client-blocked" ||
    c === "group-block" ||
    c === "group-blocked" ||
    r === "client-block" ||
    r === "client-blocked" ||
    r === "group-block" ||
    r === "group-blocked";
  const isClientAllow = r === "client-allow" || c === "client-allow";
  const isBL = c.includes("blocklist") || r === "blocklist";
  const isPhishing = c.includes("phishing") || r.startsWith("ml:phish");
  const isDGA = c.includes("dga") || r.startsWith("ml:dga");
  const isMalware = c.includes("malware") || r.startsWith("ml:malware");
  const isML = r.startsWith("ml:");

  const v = (name: string) => `var(--color-reason-${name})`;
  if (isServiceBlock || isClientBlock)
    return { bg: v("block-bg"), fg: v("block-fg"), border: v("block-border") };
  if (isClientAllow)
    return { bg: v("allow-bg"), fg: v("allow-fg"), border: v("allow-border") };
  if (isBL)
    return {
      bg: v("blocklist-bg"),
      fg: v("blocklist-fg"),
      border: v("blocklist-border"),
    };
  if (isPhishing)
    return {
      bg: v("phishing-bg"),
      fg: v("phishing-fg"),
      border: v("phishing-border"),
    };
  if (isDGA)
    return { bg: v("dga-bg"), fg: v("dga-fg"), border: v("dga-border") };
  if (isMalware)
    return {
      bg: v("malware-bg"),
      fg: v("malware-fg"),
      border: v("malware-border"),
    };
  if (isML) return { bg: v("ml-bg"), fg: v("ml-fg"), border: v("ml-border") };
  return null;
}

/**
 * Validates adblock-style filtering rules.
 * Returns an array of { line, text, error } for any invalid lines.
 * Valid patterns:
 *   ||domain^          block
 *   @@||domain^        allow
 *   127.0.0.1 domain   hosts redirect
 *   ! comment / # comment
 *   blank lines
 */
export function validateRules(
  rules: string,
): { line: number; text: string; error: string }[] {
  const errors: { line: number; text: string; error: string }[] = [];
  const domainRe =
    /^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$/;
  const ipRe = /^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}$/;

  rules.split("\n").forEach((raw, idx) => {
    const text = raw.trim();
    if (!text || text.startsWith("!") || text.startsWith("#")) return;

    // hosts-style: ip domain
    if (ipRe.test(text.split(/\s+/)[0])) {
      const parts = text.split(/\s+/);
      if (parts.length < 2) {
        errors.push({
          line: idx + 1,
          text: raw,
          error: "Missing domain after IP",
        });
      } else if (!domainRe.test(parts[1])) {
        errors.push({
          line: idx + 1,
          text: raw,
          error: `Invalid domain: "${parts[1]}"`,
        });
      }
      return;
    }

    // adblock-style
    let rest = text;
    if (rest.startsWith("@@")) rest = rest.slice(2);
    if (!rest.startsWith("||")) {
      errors.push({
        line: idx + 1,
        text: raw,
        error: 'Rule must start with "||" or "@@||"',
      });
      return;
    }
    rest = rest.slice(2);
    if (!rest.endsWith("^")) {
      errors.push({
        line: idx + 1,
        text: raw,
        error: 'Rule must end with "^"',
      });
      return;
    }
    const domain = rest.slice(0, -1);
    if (!domainRe.test(domain)) {
      errors.push({
        line: idx + 1,
        text: raw,
        error: `Invalid domain: "${domain}"`,
      });
    }
  });

  return errors;
}

export function buildDNSInstructions(
  ip: string,
): Record<string, DNSInstructionSet> {
  return {
    windows: {
      label: "Windows",
      steps: [
        { text: "Run in an elevated PowerShell prompt:" },
        {
          code: `$adapter = Get-NetAdapter | Where-Object {$_.Status -eq 'Up'} | Select-Object -First 1\nSet-DnsClientServerAddress -InterfaceAlias $adapter.Name -ServerAddresses ("${ip}")`,
        },
      ],
      note: `Or go to Network Settings → Change adapter options → IPv4 Properties and set the Preferred DNS to ${ip}.`,
    },
    mac: {
      label: "macOS",
      steps: [
        {
          text: "Run in Terminal (replace Wi-Fi with your interface name if different):",
        },
        { code: `sudo networksetup -setdnsservers Wi-Fi ${ip}` },
        { text: "To revert:" },
        { code: `sudo networksetup -setdnsservers Wi-Fi Empty` },
      ],
      note: "Or go to System Settings → Network → Wi-Fi → Details → DNS.",
    },
    linux: {
      label: "Linux",
      steps: [
        { text: "For systemd-resolved (most modern distros):" },
        {
          code: `sudo mkdir -p /etc/systemd/resolved.conf.d\nprintf '[Resolve]\\nDNS=${ip}\\n' | sudo tee /etc/systemd/resolved.conf.d/guardian.conf\nsudo systemctl restart systemd-resolved`,
        },
        { text: "Or edit /etc/resolv.conf directly:" },
        { code: `echo "nameserver ${ip}" | sudo tee /etc/resolv.conf` },
      ],
    },
    ios: {
      label: "iOS",
      steps: [
        {
          text: "Go to Settings → Wi-Fi, tap the (i) next to your network, scroll to DNS, tap Configure DNS → Manual, remove existing servers and add:",
        },
        { code: ip },
      ],
      note: "This applies only to the current Wi-Fi network.",
    },
    android: {
      label: "Android",
      steps: [
        {
          text: "Go to Settings → Network & Internet → Wi-Fi, long-press your network → Modify network → Advanced → IP settings: Static, then set DNS 1 to:",
        },
        { code: ip },
      ],
      note: "Steps vary by Android version and manufacturer.",
    },
    other: {
      label: "Router",
      steps: [
        {
          text: "Log into your router admin panel (usually 192.168.1.1 or 192.168.0.1) and set the primary DNS server to:",
        },
        { code: ip },
      ],
      note: "Once set on the router, all devices on your network will use Guardian AI automatically.",
    },
  };
}

/**
 * Returns a human-readable label for a query block/allow reason.
 * Returns null for non-actionable reasons (allowed, safe, unknown).
 */
export function reasonLabel(
  category: string | undefined,
  reason: string | undefined,
): string | null {
  const cat = (category ?? "").toLowerCase();
  const raw = (reason || category || "").toLowerCase();
  if (
    !raw ||
    ["n/a", "allowed", "safe", "safe (low confidence)", "unknown"].includes(raw)
  )
    return null;
  if (raw.startsWith("service:")) return "Service block";
  if (raw === "service") return "Service block";
  if (raw === "service-block") return "Service block";
  if (raw === "group-blocked" || raw === "group-block") return "Group block";
  if (raw === "client-blocked" || raw === "client-block") return "Client block";
  if (raw === "client-allow" || raw === "group-allow") return "Allow rule";
  if (raw === "blocklist" || cat.includes("blocklist")) return "Blocklist";
  if (raw.startsWith("ml:")) {
    const mlCat = raw.slice(3);
    return mlCat.charAt(0).toUpperCase() + mlCat.slice(1);
  }
  return category ?? null;
}

/** Returns the alternating row background colour for a query table row. */
export function normalizeBlockReason(reason: string | undefined): string {
  const raw = (reason ?? "").toLowerCase();
  if (!raw) return "";
  if (
    raw.startsWith("service:") ||
    raw === "service" ||
    raw === "service-block"
  ) {
    return "service-block";
  }
  return raw;
}

export function rowBg(blocked: boolean, index: number): string {
  if (blocked)
    return index % 2 === 0
      ? "var(--color-row-blocked)"
      : "var(--color-row-blocked-alt)";
  return index % 2 === 0 ? "var(--color-row)" : "var(--color-row-alt)";
}
