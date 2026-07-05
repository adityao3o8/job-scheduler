"use client";

import { usePathname } from "next/navigation";
import { AppShell } from "@/components/shell/AppShell";
import { AuthGuard } from "@/components/auth/AuthGuard";

export function ShellWrapper({ children }: { children: React.ReactNode }) {
  const path = usePathname();
  return (
    <AuthGuard>
      {path === "/login" ? children : <AppShell>{children}</AppShell>}
    </AuthGuard>
  );
}
