type BrowserLogLevel = "warn" | "error";

interface BrowserLogPayload {
  level?: BrowserLogLevel;
  page?: string;
  userAgent?: string;
  timestamp?: string;
  args?: string[];
}

const MAX_ARGS = 12;
const MAX_LINE_LENGTH = 16000;

function truncateLine(input: string): string {
  if (input.length <= MAX_LINE_LENGTH) {
    return input;
  }
  return `${input.slice(0, MAX_LINE_LENGTH)}…`;
}

function normalizeArgs(args: unknown): string[] {
  if (!Array.isArray(args)) {
    return [];
  }
  return args
    .slice(0, MAX_ARGS)
    .map((item) => (typeof item === "string" ? item : String(item)));
}

export async function POST(request: Request) {
  if (process.env.NEXT_PUBLIC_DEV_BROWSER_LOG_BRIDGE !== "1") {
    return new Response(null, { status: 404 });
  }

  try {
    const payload = (await request.json()) as BrowserLogPayload;
    const level = payload.level === "error" ? "error" : "warn";
    const timestamp = payload.timestamp || new Date().toISOString();
    const page = payload.page || "unknown";
    const userAgent = payload.userAgent || "unknown";
    const args = normalizeArgs(payload.args);
    const message = truncateLine(
      [
        "[browser-log]",
        level.toUpperCase(),
        timestamp,
        page,
        userAgent,
        ...args,
      ].join(" | "),
    );

    if (level === "error") {
      console.error(message);
    } else {
      console.warn(message);
    }

    return new Response(null, { status: 204 });
  } catch (error) {
    const message = error instanceof Error ? error.message : "Unexpected error";
    console.warn(`[browser-log] WARN route-parse-failed | ${message}`);
    return new Response(null, { status: 204 });
  }
}
