import { describe, expect, it } from "vitest";
import type { Finding, Severity } from "../api/types";
import {
  NO_FILTERS,
  countBySeverity,
  filterFindings,
  groupBySeverity,
  hasActiveFilters,
  toggleSeverity,
  uniqueSkills,
} from "./findings";

function finding(id: string, severity: Severity, skillId = "go", skillName = "Go"): Finding {
  return {
    id,
    skillId,
    skillName,
    checkId: "check",
    checkTitle: "Check",
    source: "deterministic",
    severity,
    confidence: 1,
    file: "main.go",
    line: 1,
    endLine: null,
    evidence: "x",
    found: "x",
    why: "x",
    improvement: "x",
    lessonId: null,
  };
}

const sample = [
  finding("1", "medium"),
  finding("2", "critical", "security", "Security"),
  finding("3", "high"),
  finding("4", "medium", "solid", "SOLID"),
];

describe("findings helpers", () => {
  it("counts by severity", () => {
    expect(countBySeverity(sample)).toEqual({ critical: 1, high: 1, medium: 2, suggestion: 0 });
  });

  it("groups by severity, most serious first, skipping empty groups", () => {
    const groups = groupBySeverity(sample);
    expect(groups.map((g) => g.severity)).toEqual(["critical", "high", "medium"]);
    expect(groups[2]?.findings.map((f) => f.id)).toEqual(["1", "4"]);
  });

  it("filters by severity and skill", () => {
    expect(filterFindings(sample, NO_FILTERS)).toHaveLength(4);
    expect(filterFindings(sample, { severities: ["medium"], skillId: null }).map((f) => f.id)).toEqual(["1", "4"]);
    expect(filterFindings(sample, { severities: ["medium"], skillId: "solid" }).map((f) => f.id)).toEqual(["4"]);
    expect(filterFindings(sample, { severities: [], skillId: "security" }).map((f) => f.id)).toEqual(["2"]);
  });

  it("lists each skill once with its finding count", () => {
    expect(uniqueSkills(sample).map((s) => [s.name, s.count])).toEqual([
      ["Go", 2],
      ["Security", 1],
      ["SOLID", 1],
    ]);
  });

  it("puts the language skill first, then REST, then SOLID, whatever order findings arrive in", () => {
    const mixed = [
      finding("1", "medium", "solid", "SOLID"),
      finding("2", "medium", "rest", "REST API Design"),
      finding("3", "medium", "typescript", "TypeScript"),
      finding("4", "medium", "solid", "SOLID"),
    ];
    const skills = uniqueSkills(mixed);
    expect(skills.map((s) => s.id)).toEqual(["typescript", "rest", "solid"]);
    expect(skills.map((s) => s.group)).toEqual(["language", "design", "design"]);
  });

  it("puts the user's own standards last, in their own group", () => {
    const skills = uniqueSkills([
      finding("1", "medium", "custom", "Your standards"),
      finding("2", "medium", "solid", "SOLID"),
      finding("3", "medium", "go", "Go"),
    ]);
    expect(skills.map((s) => s.id)).toEqual(["go", "solid", "custom"]);
    expect(skills.map((s) => s.group)).toEqual(["language", "design", "standards"]);
  });

  it("orders several language skills alphabetically, ahead of the design skills", () => {
    const skills = uniqueSkills([
      finding("1", "medium", "rest", "REST API Design"),
      finding("2", "medium", "typescript", "TypeScript"),
      finding("3", "medium", "go", "Go"),
    ]);
    expect(skills.map((s) => s.id)).toEqual(["go", "typescript", "rest"]);
  });

  it("toggles severities on and off", () => {
    const on = toggleSeverity(NO_FILTERS, "high");
    expect(on.severities).toEqual(["high"]);
    expect(hasActiveFilters(on)).toBe(true);
    const off = toggleSeverity(on, "high");
    expect(off.severities).toEqual([]);
    expect(hasActiveFilters(off)).toBe(false);
  });
});
