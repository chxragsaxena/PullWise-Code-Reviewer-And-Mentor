import { describe, expect, it } from "vitest";
import { parsePrUrl } from "./prUrl";

describe("parsePrUrl", () => {
  it.each([
    ["canonical", "https://github.com/company/project/pull/123", { owner: "company", repo: "project", number: 123 }],
    ["no scheme", "github.com/chirag/project/pull/42", { owner: "chirag", repo: "project", number: 42 }],
    ["www host", "https://www.github.com/o/r/pull/7", { owner: "o", repo: "r", number: 7 }],
    ["files tab", "https://github.com/o/r/pull/7/files", { owner: "o", repo: "r", number: 7 }],
    ["query and fragment", "https://github.com/o/r/pull/7?diff=split#discussion_r1", { owner: "o", repo: "r", number: 7 }],
    ["whitespace", "  https://github.com/o/r/pull/7\n", { owner: "o", repo: "r", number: 7 }],
  ])("accepts %s", (_name, input, expected) => {
    expect(parsePrUrl(input)).toEqual(expected);
  });

  it.each([
    ["empty", ""],
    ["not a url", "hello world"],
    ["wrong host", "https://gitlab.com/o/r/pull/7"],
    ["lookalike host", "https://github.com.evil.io/o/r/pull/7"],
    ["issue link", "https://github.com/o/r/issues/7"],
    ["missing number", "https://github.com/o/r/pull/"],
    ["non numeric", "https://github.com/o/r/pull/abc"],
    ["zero", "https://github.com/o/r/pull/0"],
    ["negative", "https://github.com/o/r/pull/-3"],
    ["ftp scheme", "ftp://github.com/o/r/pull/7"],
  ])("rejects %s", (_name, input) => {
    expect(parsePrUrl(input)).toBeNull();
  });
});
