import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

const text = (path: string): string => readFileSync(path, "utf8");

describe("penguin brand assets", () => {
  it("uses the text-free penguin source on both product surfaces without the old raster logo", () => {
    for (const file of ["src/components/Sidebar.tsx", "src/features/auth/LoginPage.tsx"]) {
      const source = text(file);
      expect(source).toContain('src="./penguin.svg"');
      expect(source).not.toMatch(/icon-192\.png|Oops-web-Omo|cat\.(?:svg|png)/i);
    }
    const login = text("src/features/auth/LoginPage.tsx");
    expect(login.match(/t\("app.name"\)/g)).toHaveLength(1);
    expect(text("public/penguin.svg")).not.toMatch(/<text\b|Oops-web-Omo/i);
    expect(text("src/i18n/locales/ko.json")).toContain('"app.name": "AI 작업실"');
    expect(text("src/i18n/locales/en.json")).toContain('"app.name": "AI Workspace"');
  });

  it("ships every manifest and HTML icon at its declared PNG size", () => {
    const manifest = JSON.parse(text("public/manifest.json")) as {
      icons: { src: string; sizes: string; type: string; purpose?: string }[];
    };
    const assertPng = (file: string, size: number): void => {
      const bytes = readFileSync(file);
      expect(Array.from(bytes.subarray(0, 8))).toEqual([137, 80, 78, 71, 13, 10, 26, 10]);
      const view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
      expect(view.getUint32(16)).toBe(size);
      expect(view.getUint32(20)).toBe(size);
    };
    expect(manifest.icons.some(icon => icon.purpose === "maskable")).toBe(true);
    for (const icon of manifest.icons) {
      expect(icon.type).toBe("image/png");
      assertPng(`public/${icon.src.replace(/^\.\//, "")}`, Number(icon.sizes.split("x")[0]));
    }
    const html = text("index.html");
    for (const [file, size] of [["favicon-16x16.png", 16], ["favicon-32x32.png", 32], ["apple-touch-icon.png", 180]] as const) {
      expect(html).toContain(`href="./${file}"`);
      assertPng(`public/${file}`, size);
    }
    expect(html).toContain('href="./favicon.ico"');
    const ico = readFileSync("public/favicon.ico");
    const view = new DataView(ico.buffer, ico.byteOffset, ico.byteLength);
    expect(view.getUint16(2, true)).toBe(1);
    expect(view.getUint16(4, true)).toBe(2);
  });
});
