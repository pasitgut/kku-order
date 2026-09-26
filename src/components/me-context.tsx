"use client";

import Image from "next/image";
import { createContext, useCallback, useContext, useEffect, useState } from "react";
import { Me, getMe, getMyPhotoUrl } from "@/lib/api";
import { initials } from "@/lib/person";

type MeState = { me: Me | null; refresh: () => void; update: (patch: Partial<Me>) => void };

const MeContext = createContext<MeState>({ me: null, refresh: () => {}, update: () => {} });

export function MeProvider({ children }: { children: React.ReactNode }) {
  const [me, setMe] = useState<Me | null>(null);

  const refresh = useCallback(() => {
    getMe().then((response) => setMe(response.data)).catch(() => setMe(null));
  }, []);
  const update = useCallback((patch: Partial<Me>) => setMe((current) => current ? { ...current, ...patch } : current), []);

  useEffect(() => { refresh(); }, [refresh]);

  return <MeContext.Provider value={{ me, refresh, update }}>{children}</MeContext.Provider>;
}

export function useMe() {
  return useContext(MeContext);
}

export function Avatar({ me, size, className = "" }: { me: Me | null; size: number; className?: string }) {
  const [brokenVersion, setBrokenVersion] = useState<number | null>(null);
  const style = { width: size, height: size, fontSize: Math.round(size * 0.36) };
  if (me?.hasPhoto && brokenVersion !== me.photoVersion) {
    return <Image unoptimized src={getMyPhotoUrl(me.photoVersion)} alt="" width={size} height={size} onError={() => setBrokenVersion(me.photoVersion)} className={`shrink-0 rounded-full object-cover ${className}`} style={style} />;
  }
  return <span aria-hidden="true" className={`flex shrink-0 items-center justify-center rounded-full bg-[var(--accent)] font-semibold text-white ${className}`} style={style}>{me ? initials(me.displayName) : ""}</span>;
}
