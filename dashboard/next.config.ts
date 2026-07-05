import type { NextConfig } from "next";
import path from "path";

const nextConfig: NextConfig = {
  outputFileTracingRoot: path.join(__dirname),
  async rewrites() {
    const api = process.env.NEXT_PUBLIC_API_URL;
    if (!api) {
      // Local dev fallback only — set NEXT_PUBLIC_API_URL on Vercel to your public API.
      return [
        {
          source: "/api/:path*",
          destination: "http://localhost:8080/:path*",
        },
      ];
    }
    return [
      {
        source: "/api/:path*",
        destination: `${api}/:path*`,
      },
    ];
  },
};

export default nextConfig;
