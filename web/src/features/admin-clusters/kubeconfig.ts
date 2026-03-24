import { load } from "js-yaml";

interface ParsedKubeconfig {
  clusters?: Array<{
    cluster?: {
      server?: unknown;
    };
  }>;
}

export function extractKubeconfigServer(kubeconfigText: string): string {
  let parsed: unknown;
  try {
    parsed = load(kubeconfigText);
  } catch (error) {
    const message =
      error instanceof Error ? error.message : "Invalid kubeconfig YAML";
    throw new Error(message);
  }

  const document = parsed as ParsedKubeconfig | null;
  const server = document?.clusters?.[0]?.cluster?.server;
  if (typeof server !== "string" || server.trim() === "") {
    throw new Error("kubeconfig must include clusters[0].cluster.server");
  }
  return server.trim();
}

export function encodeKubeconfigForTransport(value: string): string {
  const maybeBuffer = (
    globalThis as typeof globalThis & {
      Buffer?: {
        from(
          input: string,
          encoding: string,
        ): { toString(encoding: string): string };
      };
    }
  ).Buffer;
  if (maybeBuffer) {
    return maybeBuffer.from(value, "utf-8").toString("base64");
  }

  const bytes = new TextEncoder().encode(value);
  let binary = "";
  const chunkSize = 0x8000;
  for (let index = 0; index < bytes.length; index += chunkSize) {
    binary += String.fromCharCode(...bytes.subarray(index, index + chunkSize));
  }
  return btoa(binary);
}
