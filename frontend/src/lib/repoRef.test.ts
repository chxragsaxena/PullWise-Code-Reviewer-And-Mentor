import { describe, expect, it } from "vitest";
import { parseRepoRef, repoName } from "./repoRef";

describe("parseRepoRef", () => {
  it.each([
    ["golang/go", "golang/go"],
    ["  golang/go  ", "golang/go"],
    ["https://github.com/golang/go", "golang/go"],
    ["https://github.com/golang/go/", "golang/go"],
    ["https://github.com/golang/go.git", "golang/go"],
    ["http://www.github.com/golang/go/tree/master/src", "golang/go"],
    ["github.com/golang/go/pull/12", "golang/go"],
    ["go-chi/chi", "go-chi/chi"],
    ["a.b/c_d-e.f", "a.b/c_d-e.f"],
    ["owner/name/extra/path", "owner/name"],
  ])("accepts %s", (input, want) => {
    const ref = parseRepoRef(input);
    expect(ref && repoName(ref)).toBe(want);
  });

  it.each([
    "",
    "   ",
    "golang",
    "/golang/go",
    "golang//go",
    "https://gitlab.com/golang/go",
    "https://github.com/golang",
    "https://evil.example/github.com/golang/go",
    "golang/go space",
    "../etc/passwd",
    "./repo",
    "owner/..",
  ])("rejects %j", (input) => {
    expect(parseRepoRef(input)).toBeNull();
  });
});
