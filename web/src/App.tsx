import { Routes, Route, Navigate } from "react-router-dom";
import { AuthProvider } from "@/auth/AuthProvider";
import { RequireAuth, RequireAdmin, RequireAnon } from "@/auth/guards";
import { Layout } from "@/components/Layout";
import { Setup } from "@/pages/Setup";
import { Login } from "@/pages/Login";
import { TotpEnroll } from "@/pages/TotpEnroll";
import { TotpVerify } from "@/pages/TotpVerify";
import { Sites } from "@/pages/Sites";
import { SiteForm } from "@/pages/SiteForm";
import { Ssl } from "@/pages/Ssl";
import { Users } from "@/pages/Users";
import { ChangePassword } from "@/pages/ChangePassword";
import { Logs } from "@/pages/Logs";

export default function App() {
  return (
    <AuthProvider>
      <Routes>
        <Route element={<RequireAnon mode="setup" />}>
          <Route path="/setup" element={<Setup />} />
        </Route>
        <Route element={<RequireAnon mode="login" />}>
          <Route path="/login" element={<Login />} />
        </Route>
        <Route path="/totp/enroll" element={<TotpEnroll />} />
        <Route path="/totp/verify" element={<TotpVerify />} />

        <Route element={<RequireAuth />}>
          <Route path="/app" element={<Layout />}>
            <Route index element={<Navigate to="/app/sites" replace />} />
            <Route path="sites" element={<Sites />} />
            <Route path="sites/new" element={<SiteForm />} />
            <Route path="sites/:id/edit" element={<SiteForm />} />
            <Route path="ssl" element={<Ssl />} />
            <Route path="logs" element={<Logs />} />
            <Route path="me" element={<ChangePassword />} />
            <Route element={<RequireAdmin />}>
              <Route path="users" element={<Users />} />
            </Route>
          </Route>
        </Route>

        <Route path="*" element={<Navigate to="/app/sites" replace />} />
      </Routes>
    </AuthProvider>
  );
}
