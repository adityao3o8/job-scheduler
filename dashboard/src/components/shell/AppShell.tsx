"use client";

import { LeftRail } from "./LeftRail";
import { TopBar } from "./TopBar";
import { ConsoleProvider } from "@/components/providers/ConsoleProvider";
import { CommandPalette } from "@/components/ui/CommandPalette";
import { WorkerPulse } from "./WorkerPulse";

export function AppShell({ children }: { children: React.ReactNode }) {
  return (
    <ConsoleProvider>
      <WorkerPulse />
      <div className="flex h-screen overflow-hidden" style={{ background: "var(--bg-void)" }}>
        <LeftRail />
        <div className="flex flex-col flex-1 min-w-0">
          <TopBar />
          <main
            className="flex-1 overflow-y-auto pb-20 md:pb-0"
            style={{ padding: "24px" }}
          >
            <div style={{ maxWidth: "var(--content-max)", margin: "0 auto" }}>
              {children}
            </div>
          </main>
        </div>
      </div>
      <CommandPalette />
    </ConsoleProvider>
  );
}
