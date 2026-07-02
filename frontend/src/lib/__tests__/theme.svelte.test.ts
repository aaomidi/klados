import {describe, it, expect, beforeEach} from "vitest";
import {setTheme, getTheme, getResolved} from "$lib/theme.svelte";

// Theme selection used to live behind a Header button (removed when the
// setting moved to GeneralSettings); this covers the module directly.
describe("theme", () => {
  beforeEach(() => {
    setTheme("system");
    document.documentElement.classList.remove("light", "dark");
  });

  it("stores the selected theme", () => {
    expect(getTheme()).toBe("system");
    setTheme("dark");
    expect(getTheme()).toBe("dark");
    setTheme("light");
    expect(getTheme()).toBe("light");
  });

  it("applies an explicit theme as a class on <html> and clears the others", () => {
    setTheme("dark");
    expect(document.documentElement.classList.contains("dark")).toBe(true);
    expect(document.documentElement.classList.contains("light")).toBe(false);

    setTheme("light");
    expect(document.documentElement.classList.contains("light")).toBe(true);
    expect(document.documentElement.classList.contains("dark")).toBe(false);
  });

  it("adds no class for system (falls through to the media query)", () => {
    setTheme("dark");
    setTheme("system");
    expect(document.documentElement.classList.contains("dark")).toBe(false);
    expect(document.documentElement.classList.contains("light")).toBe(false);
  });

  it("resolves an explicit theme verbatim", () => {
    setTheme("dark");
    expect(getResolved()).toBe("dark");
    setTheme("light");
    expect(getResolved()).toBe("light");
  });

  it("resolves system to a concrete light/dark value", () => {
    setTheme("system");
    expect(["light", "dark"]).toContain(getResolved());
  });
});
