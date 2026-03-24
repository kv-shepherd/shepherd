import { describe, expect, it } from "vitest";

import {
  encodeKubeconfigForTransport,
  extractKubeconfigServer,
} from "./kubeconfig";

describe("admin cluster kubeconfig helpers", () => {
  it("extracts the first cluster server from kubeconfig yaml", () => {
    const server = extractKubeconfigServer(
      [
        "apiVersion: v1",
        "kind: Config",
        "clusters:",
        "  - name: test",
        "    cluster:",
        "      server: https://cluster.example.com",
      ].join("\n"),
    );

    expect(server).toBe("https://cluster.example.com");
  });

  it("rejects kubeconfig yaml without clusters[0].cluster.server", () => {
    expect(() =>
      extractKubeconfigServer(
        ["apiVersion: v1", "kind: Config", "clusters: []"].join("\n"),
      ),
    ).toThrow("kubeconfig must include clusters[0].cluster.server");
  });

  it("encodes kubeconfig text as base64 transport bytes", () => {
    expect(encodeKubeconfigForTransport("hello")).toBe("aGVsbG8=");
  });
});
