import { createContext, useContext, useEffect, type ReactNode } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "react-router-dom";
import { api, authEvents } from "@/api/client";
import type { Me } from "@/api/types";

export type AuthState =
  | "loading"
  | "needsSetup"
  | "unauthenticated"
  | "authenticated";

interface AuthCtx {
  state: AuthState;
  me: Me["user"] | undefined;
  refresh: () => void;
  logout: () => Promise<void>;
}

const Ctx = createContext<AuthCtx | null>(null);

async function fetchMe(): Promise<Me> {
  const { data } = await api.get<Me>("/auth/me");
  return data;
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const navigate = useNavigate();
  const qc = useQueryClient();

  const { data, isLoading } = useQuery({
    queryKey: ["me"],
    queryFn: fetchMe,
    staleTime: 10_000,
    retry: false,
  });

  useEffect(() => {
    const offUnauth = authEvents.on("unauthorized", () => {
      qc.setQueryData<Me>(["me"], { needsSetup: false, authenticated: false });
      navigate("/login", { replace: true });
    });
    const offTotp = authEvents.on("needs_totp", () => navigate("/login", { replace: true }));
    const offSetup = authEvents.on("needs_setup", () => navigate("/setup", { replace: true }));
    return () => {
      offUnauth();
      offTotp();
      offSetup();
    };
  }, [navigate, qc]);

  let state: AuthState = "loading";
  if (!isLoading) {
    if (data?.needsSetup) state = "needsSetup";
    else if (data?.authenticated) state = "authenticated";
    else state = "unauthenticated"; // includes the network-error case (data undefined)
  }

  const value: AuthCtx = {
    state,
    me: data?.user,
    refresh: () => qc.invalidateQueries({ queryKey: ["me"] }),
    logout: async () => {
      try {
        await api.post("/auth/logout");
      } catch {
        /* ignore */
      }
      qc.setQueryData<Me>(["me"], { needsSetup: false, authenticated: false });
      navigate("/login", { replace: true });
    },
  };

  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useAuth() {
  const ctx = useContext(Ctx);
  if (!ctx) throw new Error("useAuth must be used within AuthProvider");
  return ctx;
}
