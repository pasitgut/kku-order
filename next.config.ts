import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  async rewrites() {
    return [
      {
        source: "/backend/:path*",
        destination: `${process.env.BACKEND_INTERNAL_URL ?? "http://localhost:8080"}/:path*`,
      },
    ];
  },
};

export default nextConfig;
