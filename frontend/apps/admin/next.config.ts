import type { NextConfig } from "next";

const API_TARGET = process.env.API_TARGET || "http://localhost:8081";

const nextConfig: NextConfig = {
  async rewrites() {
    return [
      { source: "/api/v1/:path*", destination: `${API_TARGET}/api/v1/:path*` },
    ];
  },
};

export default nextConfig;
