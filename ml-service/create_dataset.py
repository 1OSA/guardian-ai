import io
import os
import random

import pandas as pd
import requests

# --- SOURCES ---
SAFE_URL = "http://downloads.majestic.com/majestic_million.csv"
DGA_URL = "https://raw.githubusercontent.com/chrmor/DGA_domains_dataset/master/dga_domains_full.csv"
OUTPUT_FILE = "guardian_dataset_full.csv"

# --- THE FULL ATTACK DICTIONARY (CYRILLIC, GREEK, LATIN) ---
# This includes the "Invisible" characters used in high-end phishing.
HOMOGLYPHS = {
    "a": [
        "\u0430",  # Cyrillic Small A (Looks identical)
        "\u03b1",  # Greek Small Alpha
        "@",
        "4",
        "\u00e0",
        "\u00e1",
        "\u00e2",
        "\u00e3",
        "\u00e4",
        "\u00e5",  # Accented
        "\u0105",
        "\u0251",
    ],
    "b": [
        "\u04b9",  # Cyrillic Small Be
        "6",
        "8",
        "\u00df",  # Latin Small Sharp S (Eszett) - looks like B
        "\u0180",  # Latin Small B with Stroke
        "l3",
        "lz",
        "lo",
    ],
    "c": [
        "\u0441",  # Cyrillic Small Es (Looks identical)
        "\u03f2",  # Greek Lunate Sigma
        "(",
        "k",
        "<",
        "\u0107",
        "\u0109",
        "\u010b",
        "\u010d",
    ],
    "d": [
        "\u0501",  # Cyrillic Small Dwe
        "b",
        "cl",
        "ol",
        "\u0257",  # Latin Small D with Hook
        "\u00f0",  # Latin Small Eth
    ],
    "e": [
        "\u0435",  # Cyrillic Small Ie (Looks identical)
        "\u04bd",  # Cyrillic Small Che with Vertical Stroke
        "\u03b5",  # Greek Small Epsilon
        "3",
        "\u00e8",
        "\u00e9",
        "\u00ea",
        "\u00eb",
        "\u0113",
        "\u0115",
    ],
    "f": [
        "\u04cf",  # Cyrillic Small Palochka
        "\u0192",  # Latin Small F with Hook
        "ph",
    ],
    "g": [
        "\u0261",  # Latin Small Script G (Looks identical to standard 'g')
        "q",
        "9",
        "6",
        "\u0121",
        "\u0123",
        "\u011f",
    ],
    "h": [
        "\u04bb",  # Cyrillic Small Shha
        "\u043d",  # Cyrillic Small En (Looks like small caps 'H')
        "n",
        "ln",
        "lr",
    ],
    "i": [
        "\u0456",  # Cyrillic Small Byelorussian-Ukrainian I (Identical)
        "\u04cf",  # Cyrillic Small Palochka
        "1",
        "l",
        "!",
        "|",
        "j",
        "\u00ec",
        "\u00ed",
        "\u00ee",
        "\u00ef",
        "\u0129",
        "\u012b",
    ],
    "j": [
        "\u0458",  # Cyrillic Small Je
        "\u029d",  # Latin Small J with Crossed-Tail
        "i",
        ";",
        "y",
    ],
    "k": [
        "\u03ba",  # Greek Small Kappa
        "\u043a",  # Cyrillic Small Ka
        "lc",
        "lk",
        "1k",
    ],
    "l": [
        "1",
        "I",  # The Classic "One / Capital Eye"
        "\u0196",  # Latin Capital Iota
        "|",
        "!",
        "\u0142",  # Latin Small L with Stroke
    ],
    "m": [
        "\u043c",  # Cyrillic Small Em (Looks like small caps M)
        "rn",
        "nn",
        "rri",
        "\u0271",  # Latin Small M with Hook
    ],
    "n": [
        "\u050b",  # Cyrillic Small En with Hook
        "\u03b7",  # Greek Small Eta
        "m",
        "r",
        "\u00f1",
        "\u0144",
    ],
    "o": [
        "\u043e",  # Cyrillic Small O (Identical)
        "\u03bf",  # Greek Small Omicron (Identical)
        "0",
        "\u00f2",
        "\u00f3",
        "\u00f4",
        "\u00f5",
        "\u00f6",
        "\u00f8",
    ],
    "p": [
        "\u0440",  # Cyrillic Small Er (Looks Identical to 'p')
        "\u03c1",  # Greek Small Rho
        "q",
    ],
    "q": [
        "\u051b",  # Cyrillic Small Qa
        "g",
        "9",
        "p",
    ],
    "r": [
        "\u0433",  # Cyrillic Small Ghe (Looks like 'r' in many fonts)
        "\u027c",  # Latin Small R with Long Leg
        "n",
        "z",
    ],
    "s": [
        "\u0455",  # Cyrillic Small Dze
        "5",
        "$",
        "z",
        "\u015b",
        "\u0161",
    ],
    "t": [
        "\u03c4",  # Greek Small Tau
        "+",
        "7",
        "\u0163",
        "\u0165",
    ],
    "u": [
        "\u0446",  # Cyrillic Small Tse
        "\u03c5",  # Greek Small Upsilon
        "v",
        "\u00fc",
        "\u00fa",
        "\u00f9",
        "\u00bb",
    ],
    "v": [
        "\u0475",  # Cyrillic Small Izhitsa
        "\u03bd",  # Greek Small Nu (Looks like 'v')
        "u",
        "\\/",
        "\u2228",
    ],
    "w": [
        "\u051d",  # Cyrillic Small We
        "\u0448",  # Cyrillic Small Sha (Looks like 'w')
        "vv",
        "uu",
        "\u0175",
    ],
    "x": [
        "\u0445",  # Cyrillic Small Ha (Identical to 'x')
        "\u03c7",  # Greek Small Chi
        "%",
        "*",
        "><",
    ],
    "y": [
        "\u0443",  # Cyrillic Small U (Identical to 'y')
        "\u03b3",  # Greek Small Gamma
        "v",
        "j",
        "\u00fd",
        "\u00ff",
    ],
    "z": [
        "\u0491",  # Cyrillic Small Ghe with Upturn
        "2",
        "s",
        "\u017a",
        "\u017c",
        "\u017e",
    ],
}


def download_file(url, filename_tag):
    print(f"Downloading {filename_tag}...")
    headers = {
        "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36"
    }
    try:
        r = requests.get(url, headers=headers, stream=True, timeout=20)
        r.raise_for_status()
        return r.content
    except Exception as e:
        print(f"\n[!] ERROR downloading {filename_tag}: {e}")
        return None


def generate_attacks(domain_list, num_needed):
    print(
        f"Generating {num_needed} forensic-grade attack domains with EXTENDED homoglyphs..."
    )
    attacks = set()
    while len(attacks) < num_needed:
        target = random.choice(domain_list)
        if "." not in target:
            continue
        name, tld = target.split(".", 1)
        if len(name) < 3:
            continue

        # Strategies:
        # idn: Swap latin chars for Cyrillic/Greek lookalikes
        # visual: Swap chars for ASCII lookalikes
        # typo: Fat finger / omission
        strategy = random.choice(["idn", "visual", "typo"])
        new_name = list(name)

        try:
            if strategy == "idn":
                swapped = False
                for i, char in enumerate(new_name):
                    # We heavily favor the first item in the list as it is usually the "Perfect" homoglyph
                    if char in HOMOGLYPHS:
                        # 60% chance to swap a swappable character
                        if random.random() > 0.4:
                            # Pick randomly from the top 2 (most realistic)
                            options = HOMOGLYPHS[char][:2]
                            new_name[i] = random.choice(options)
                            swapped = True
                if not swapped:
                    continue

            elif strategy == "visual":
                idx = random.randint(0, len(name) - 1)
                char = name[idx]
                if char in HOMOGLYPHS:
                    # Pick from the REST of the list (ASCII/Visual fakes)
                    options = HOMOGLYPHS[char][2:]
                    if options:
                        new_name[idx] = random.choice(options)

            elif strategy == "typo":
                if len(name) > 3:
                    idx = random.randint(0, len(name) - 2)
                    new_name[idx], new_name[idx + 1] = new_name[idx + 1], new_name[idx]

            final_domain = "".join(new_name) + "." + tld
            if final_domain != target:
                attacks.add(final_domain)
        except Exception:
            continue
    return list(attacks)


def process_data():
    # --- 1. SAFE DOMAINS ---
    if os.path.exists("temp_safe.csv"):
        print("Using local 'temp_safe.csv'...")
        df_safe_raw = pd.read_csv("temp_safe.csv")
    else:
        content = download_file(SAFE_URL, "Safe List")
        if not content:
            return
        df_safe_raw = pd.read_csv(io.BytesIO(content))

    top_brands = df_safe_raw["Domain"].iloc[:2500].tolist()
    safe_list = df_safe_raw["Domain"].iloc[:50000].tolist()

    # --- 2. DGA DOMAINS ---
    if os.path.exists("temp_dga.csv"):
        print("Using local 'temp_dga.csv'...")
        df_dga_raw = pd.read_csv(
            "temp_dga.csv", header=None, names=["type", "source", "domain"]
        )
    else:
        content = download_file(DGA_URL, "DGA List")
        if not content:
            return
        df_dga_raw = pd.read_csv(
            io.BytesIO(content), header=None, names=["type", "source", "domain"]
        )

    dga_list = df_dga_raw[df_dga_raw["type"] == "dga"]["domain"].tolist()
    random.shuffle(dga_list)
    dga_list = dga_list[:50000]

    # --- 3. GENERATE ATTACKS ---
    attack_list = generate_attacks(top_brands, 50000)

    # --- 4. MERGE ---
    print(
        f"Merging: {len(safe_list)} Safe + {len(dga_list)} DGA + {len(attack_list)} Attacks"
    )

    # 0 = Safe
    # 1 = DGA
    # 2 = Phishing/IDN
    df_safe = pd.DataFrame({"domain": safe_list, "label": 0})
    df_dga = pd.DataFrame({"domain": dga_list, "label": 1})
    df_attack = pd.DataFrame({"domain": attack_list, "label": 2})

    df_final = pd.concat([df_safe, df_dga, df_attack], ignore_index=True)
    df_final = df_final.sample(frac=1).reset_index(drop=True)

    # --- 5. SAVE ---
    # encoding='utf-8' is MANDATORY for Cyrillic/Greek characters
    df_final.to_csv(OUTPUT_FILE, index=False, encoding="utf-8")
    print(f"SUCCESS: Saved {len(df_final)} rows to {OUTPUT_FILE}")


if __name__ == "__main__":
    process_data()
