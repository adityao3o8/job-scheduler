import type { Metadata } from "next";
import { GeistSans } from "geist/font/sans";
import { GeistMono } from "geist/font/mono";
import "./globals.css";
import { ShellWrapper } from "@/components/shell/ShellWrapper";

export const metadata: Metadata = {
  title: "Scheduler — Control Plane",
  description: "Distributed job scheduler operator console",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" className={`${GeistSans.variable} ${GeistMono.variable}`}>
      <body className={GeistSans.className}>
        <ShellWrapper>{children}</ShellWrapper>
      </body>
    </html>
  );
}
