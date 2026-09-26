import { describe, expect, it } from "vitest";
import { formatConfidence, formatLocation, formatRelativeTime } from "./format";

const NOW = new Date("2026-09-26T12:00:00Z").getTime();

describe("formatRelativeTime", () => {
  it.each([
    ["2026-09-26T11:59:40Z", "just now"],
    ["2026-09-26T11:55:00Z", "5 minutes ago"],
    ["2026-09-26T11:00:00Z", "1 hour ago"],
    ["2026-09-26T07:00:00Z", "5 hours ago"],
    ["2026-09-24T12:00:00Z", "2 days ago"],
  ])("%s -> %s", (iso, expected) => {
    expect(formatRelativeTime(iso, NOW)).toBe(expected);
  });

  it("returns an empty string for an invalid date", () => {
    expect(formatRelativeTime("nope", NOW)).toBe("");
  });

  it("never reports the future as negative", () => {
    expect(formatRelativeTime("2026-09-26T12:05:00Z", NOW)).toBe("just now");
  });
});

describe("formatConfidence", () => {
  it("rounds to a percentage", () => {
    expect(formatConfidence(0.824)).toBe("82%");
    expect(formatConfidence(1)).toBe("100%");
  });
});

describe("formatLocation", () => {
  it("shows the file alone, a single line or a range", () => {
    expect(formatLocation("a.go", null, null)).toBe("a.go");
    expect(formatLocation("a.go", 4, null)).toBe("a.go:4");
    expect(formatLocation("a.go", 4, 4)).toBe("a.go:4");
    expect(formatLocation("a.go", 4, 9)).toBe("a.go:4-9");
  });
});
