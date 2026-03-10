import React from "react";
import {
  FaGamepad,
  FaAmazon,
} from "react-icons/fa";
import {
  SiRoblox,
  SiNetflix,
  SiYoutube,
  SiTiktok,
  SiInstagram,
  SiFacebook,
  SiX,
  SiDiscord,
  SiSteam,
  SiTwitch,
  SiSnapchat,
  SiWhatsapp,
  SiSpotify,
  SiEpicgames,
  SiReddit,
  SiFortnite,
} from "react-icons/si";
import { GiStoneCrafting } from "react-icons/gi";
import { FaWandMagicSparkles } from "react-icons/fa6";

/**
 * Maps each predefined service ID to its brand icon component.
 * Falls back to FaGamepad for any unknown ID.
 * All icons are bundled by Vite — nothing is fetched at runtime.
 */
export const SERVICE_ICONS: Record<string, React.ReactElement> = {
  roblox: <SiRoblox />,
  netflix: <SiNetflix />,
  youtube: <SiYoutube />,
  tiktok: <SiTiktok />,
  instagram: <SiInstagram />,
  facebook: <SiFacebook />,
  twitter: <SiX />,
  discord: <SiDiscord />,
  steam: <SiSteam />,
  twitch: <SiTwitch />,
  snapchat: <SiSnapchat />,
  whatsapp: <SiWhatsapp />,
  spotify: <SiSpotify />,
  minecraft: <GiStoneCrafting />,
  fortnite: <SiFortnite />,
  epicgames: <SiEpicgames />,
  reddit: <SiReddit />,
  amazon_prime: <FaAmazon />,
  disneyplus: <FaWandMagicSparkles />,
};

/** Returns the brand icon for a service, or a generic gamepad fallback. */
export function getServiceIcon(id: string): React.ReactElement {
  return SERVICE_ICONS[id] ?? <FaGamepad />;
}
