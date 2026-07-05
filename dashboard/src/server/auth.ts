import { SignJWT, jwtVerify } from "jose";

export interface Claims {
  user_id: string;
  org_id: string;
  email: string;
  role: string;
}

function secret() {
  const s = process.env.JWT_SECRET || "vercel-demo-secret-change-me";
  return new TextEncoder().encode(s);
}

export async function signToken(claims: Claims): Promise<string> {
  return new SignJWT({ ...claims })
    .setProtectedHeader({ alg: "HS256" })
    .setIssuedAt()
    .setExpirationTime("24h")
    .sign(secret());
}

export async function verifyToken(token: string): Promise<Claims | null> {
  try {
    const { payload } = await jwtVerify(token, secret());
    return {
      user_id: String(payload.user_id),
      org_id: String(payload.org_id),
      email: String(payload.email),
      role: String(payload.role),
    };
  } catch {
    return null;
  }
}

export function demoAuthEnabled(): boolean {
  return process.env.DEMO_AUTH !== "false";
}

export function bearerClaims(req: Request): Claims | null {
  const auth = req.headers.get("authorization");
  if (!auth?.startsWith("Bearer ")) return null;
  return null; // async verify in route
}

export async function claimsFromRequest(req: Request): Promise<Claims | null> {
  const auth = req.headers.get("authorization");
  if (!auth?.startsWith("Bearer ")) return null;
  return verifyToken(auth.slice(7));
}
