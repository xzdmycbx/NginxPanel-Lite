import { Navigate, Outlet } from "react-router-dom";
import { useAuth } from "./AuthProvider";
import { FullPageSpinner } from "@/components/ui/spinner";

export function RequireAuth() {
  const { state } = useAuth();
  if (state === "loading") return <FullPageSpinner />;
  if (state === "needsSetup") return <Navigate to="/setup" replace />;
  if (state === "unauthenticated") return <Navigate to="/login" replace />;
  return <Outlet />;
}

export function RequireAdmin() {
  const { me } = useAuth();
  if (me?.role !== "admin") return <Navigate to="/app/sites" replace />;
  return <Outlet />;
}

/** RequireAnon is for /login and /setup: redirect away when already usable. */
export function RequireAnon({ mode }: { mode: "login" | "setup" }) {
  const { state } = useAuth();
  if (state === "loading") return <FullPageSpinner />;
  if (state === "authenticated") return <Navigate to="/app/sites" replace />;
  if (mode === "login" && state === "needsSetup") return <Navigate to="/setup" replace />;
  if (mode === "setup" && state !== "needsSetup") return <Navigate to="/login" replace />;
  return <Outlet />;
}
