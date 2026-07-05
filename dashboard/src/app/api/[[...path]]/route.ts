import { handleApi } from "@/server/router";

type Params = { params: Promise<{ path?: string[] }> };

async function dispatch(req: Request, params: Promise<{ path?: string[] }>) {
  const { path = [] } = await params;
  return handleApi(req, path);
}

export async function GET(req: Request, { params }: Params) {
  return dispatch(req, params);
}

export async function POST(req: Request, { params }: Params) {
  return dispatch(req, params);
}

export async function PUT(req: Request, { params }: Params) {
  return dispatch(req, params);
}

export async function DELETE(req: Request, { params }: Params) {
  return dispatch(req, params);
}
