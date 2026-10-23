import { afterEach, describe, expect, it, vi } from "vitest";
import { createTerminal, fsCreateFolder } from "./terminal";

describe("createTerminal", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("sends one explicit provider and never a model or resume identity", async () => {
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => new Response(
      JSON.stringify({ id: "chat-1", name: "chat", provider: "omo" }),
      { status: 201 },
    ));
    vi.stubGlobal("fetch", fetchMock);

    await createTerminal("ws-1", "", "omo");

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [path, init] = fetchMock.mock.calls[0]!;
    expect(path).toBe("/api/workspaces/ws-1/chats");
    expect(JSON.parse(String(init?.body))).toEqual({ name: "", provider: "omo" });
    expect(String(init?.body)).not.toContain("model");
    expect(String(init?.body)).not.toContain("resumeIdentity");
  });
});

describe("fsCreateFolder", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("posts the parent path and leaf name to the mkdir endpoint", async () => {
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => new Response(
      JSON.stringify({ path: "/work/new" }),
      { status: 201 },
    ));
    vi.stubGlobal("fetch", fetchMock);

    const created = await fsCreateFolder("/work", "new");

    expect(created).toEqual({ path: "/work/new" });
    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [path, init] = fetchMock.mock.calls[0]!;
    expect(path).toBe("/api/fs/mkdir");
    expect(init?.method).toBe("POST");
    expect(JSON.parse(String(init?.body))).toEqual({ path: "/work", name: "new" });
  });
});

describe("file URL encoding", () => {
  afterEach(() => { vi.unstubAllGlobals(); });
  it("opens the exact server-returned path without fragment or query truncation", async () => {
    const { fsRead, downloadUrl } = await import("./terminal");
    const { rawUrl } = await import("./filePreview");
    const fetchMock = vi.fn(async (_input: RequestInfo | URL) => new Response(JSON.stringify({ content: "file", size: 4 }), { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);
    for (const name of ["quarter#1.pdf", "query?.csv", "tail.pdf", "보고서 (1).pdf", "-proof.pdf"]) {
      const path = `/work/uploads/${name}`;
      await fsRead(path);
      const readUrl = String(fetchMock.mock.calls.at(-1)![0]);
      for (const value of [readUrl, downloadUrl(path), rawUrl(path)]) {
        const url = new URL(value, "https://workspace.invalid");
        expect(url.hash).toBe("");
        expect(url.searchParams.get("path")).toBe(path);
      }
    }
  });
});
