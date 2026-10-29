// Sample mock content; some findings use skills the backend lacks yet, to exercise the UI.

import type { Finding, Lesson, PullRequest } from "./types";

type FindingTemplate = Omit<Finding, "id">;

const TEMPLATES: readonly FindingTemplate[] = [
  {
    skillId: "security",
    skillName: "Security",
    checkId: "sql-injection",
    checkTitle: "SQL built by string concatenation",
    source: "deterministic",
    severity: "critical",
    confidence: 1,
    file: "internal/orders/repository.go",
    line: 64,
    endLine: null,
    evidence: 'rows, err := db.Query("SELECT * FROM orders WHERE customer_id = " + customerID)',
    found: "The query is assembled by concatenating customerID into the SQL text.",
    why: "Anything inside customerID becomes part of the SQL statement, so a crafted value can read or change other rows. Databases can only separate code from data if you send them separately.",
    improvement: "Pass the value as a query parameter so the database treats it as data.",
    lessonId: "sql-injection",
  },
  {
    skillId: "go",
    skillName: "Go",
    checkId: "ignored-error",
    checkTitle: "Error return value is ignored",
    source: "deterministic",
    severity: "high",
    confidence: 1,
    file: "internal/config/loader.go",
    line: 42,
    endLine: null,
    evidence: "data, _ := os.ReadFile(path)",
    found: "The error from os.ReadFile is discarded with the blank identifier.",
    why: "If the file is missing or unreadable, data is silently empty and the program keeps running with wrong configuration. The failure surfaces later, far from its cause.",
    improvement: "Return the error with context so the caller knows which file failed.",
    lessonId: "error-handling",
  },
  {
    skillId: "go",
    skillName: "Go",
    checkId: "goroutine-leak-risk",
    checkTitle: "Goroutine started with no way to stop or wait for it",
    source: "ai",
    severity: "high",
    confidence: 0.82,
    file: "internal/notify/worker.go",
    line: 33,
    endLine: 39,
    evidence: "go func() {\n\tfor msg := range ch {\n\t\thandle(msg)\n\t}\n}()",
    found: "The goroutine loops over ch, but nothing in this change closes ch or cancels a context.",
    why: "If ch is never closed, the goroutine blocks forever and keeps everything it references alive. Repeated calls accumulate leaked goroutines.",
    improvement: "Give the goroutine a context to stop on, and wait for it with an errgroup or WaitGroup.",
    lessonId: "concurrency",
  },
  {
    skillId: "solid",
    skillName: "SOLID",
    checkId: "single-responsibility",
    checkTitle: "Type has more than one reason to change",
    source: "ai",
    severity: "high",
    confidence: 0.78,
    file: "internal/orders/service.go",
    line: 21,
    endLine: 118,
    evidence: "OrderService.Create creates the order, sends the confirmation email and generates the invoice PDF.",
    found: "OrderService currently handles order creation, email notifications and invoice generation.",
    why: "These responsibilities change for different reasons. A change to invoice layout forces edits to a service that also owns order rules, raising the risk of breaking unrelated behavior.",
    improvement: "Move invoice generation and notification into their own services and let OrderService call them.",
    lessonId: "srp",
  },
  {
    skillId: "go",
    skillName: "Go",
    checkId: "context-not-first",
    checkTitle: "context.Context is not the first parameter",
    source: "deterministic",
    severity: "medium",
    confidence: 1,
    file: "internal/orders/service.go",
    line: 18,
    endLine: null,
    evidence: "func Fetch(id int, ctx context.Context) error",
    found: "ctx is the second parameter of Fetch.",
    why: "Go code conventionally takes ctx first. Tools, linters and readers rely on that, and mixed positions make call sites easy to get wrong.",
    improvement: "Move ctx to the first parameter: func Fetch(ctx context.Context, id int) error.",
    lessonId: "context",
  },
  {
    skillId: "go",
    skillName: "Go",
    checkId: "http-call-without-context",
    checkTitle: "HTTP or database call made without a context",
    source: "deterministic",
    severity: "medium",
    confidence: 1,
    file: "internal/payments/client.go",
    line: 57,
    endLine: null,
    evidence: "resp, err := http.Get(url)",
    found: "http.Get has no context, so the request cannot be cancelled or given a deadline.",
    why: "If the caller goes away or the remote service stalls, this request keeps running and holds a connection open.",
    improvement: "Build the request with http.NewRequestWithContext and pass the caller's ctx.",
    lessonId: "context",
  },
  {
    skillId: "go",
    skillName: "Go",
    checkId: "high-complexity",
    checkTitle: "Function has high cyclomatic complexity",
    source: "deterministic",
    severity: "medium",
    confidence: 1,
    file: "internal/orders/handler.go",
    line: 91,
    endLine: 160,
    evidence: "Cyclomatic complexity of handleCreate is 18 (threshold 15).",
    found: "handleCreate has 18 independent paths through it.",
    why: "Each extra branch is another case to test and hold in your head. Functions this size hide bugs in rarely taken paths.",
    improvement: "Extract validation and response mapping into small named functions.",
    lessonId: "error-handling",
  },
  {
    skillId: "go",
    skillName: "Go",
    checkId: "interface-too-large",
    checkTitle: "Interface is larger than its consumers need",
    source: "ai",
    severity: "suggestion",
    confidence: 0.74,
    file: "internal/users/repository.go",
    line: 12,
    endLine: 30,
    evidence: "type UserRepository interface {\n\tGet(...)\n\tList(...)\n\tCreate(...)\n\t// 9 more methods\n}",
    found: "UserRepository declares 12 methods, but its only caller in this change uses Get.",
    why: "Large interfaces make fakes expensive to write and couple callers to methods they never use.",
    improvement: "Declare a one-method interface in the package that calls Get.",
    lessonId: "interfaces",
  },
];

/** Findings for a review. Varies by PR number so different reviews look different. */
export function buildFindings(reviewId: string, prNumber: number): Finding[] {
  return TEMPLATES.filter((_, index) => (index + prNumber) % 4 !== 0).map((template, index) => ({
    ...template,
    id: `${reviewId}-f${index + 1}`,
  }));
}

const TITLES = [
  "Add order caching",
  "Refactor payment client",
  "Introduce notification worker",
  "Fix config loading on startup",
  "Split order handler",
];
const AUTHORS = ["chirag", "maya-dev", "rjensen", "alex-k"];

/** Deterministic fake metadata for a PR link. */
export function buildPullRequest(owner: string, repo: string, number: number): PullRequest {
  return {
    owner,
    repo,
    number,
    title: TITLES[number % TITLES.length] ?? "Update code",
    author: AUTHORS[number % AUTHORS.length] ?? owner,
    url: `https://github.com/${owner}/${repo}/pull/${number}`,
    headSha: (number * 2654435761).toString(16).padStart(40, "a").slice(-40),
    filesChanged: 3 + (number % 9),
    additions: 40 + ((number * 37) % 400),
    deletions: 5 + ((number * 13) % 120),
  };
}

export const LESSONS: Record<string, Lesson> = {
  "error-handling": {
    id: "error-handling",
    markdown: `# Error handling in Go

## Why it exists
Go returns errors as ordinary values instead of throwing exceptions. The compiler does not force you to look at them, so discipline is the only thing that stops a failure from silently turning into wrong behavior later.

## The rule
Every error is either handled, returned with added context, or deliberately ignored with a comment explaining why.

## Common mistakes
\`\`\`go
data, _ := os.ReadFile(path)   // failure becomes an empty slice
if err != nil { return err }   // loses where it failed
\`\`\`

## Better
\`\`\`go
data, err := os.ReadFile(path)
if err != nil {
    return fmt.Errorf("read config %q: %w", path, err)
}
\`\`\`

\`%w\` keeps the original error inspectable with \`errors.Is\` and \`errors.As\`.

## Try it yourself
Find a function in your PR that returns only \`error\`. Follow one call to it and answer: what happens to the caller's state if it fails halfway?`,
  },
  context: {
    id: "context",
    markdown: `# Context propagation

## Why it exists
\`context.Context\` carries cancellation, deadlines and request-scoped values across API boundaries. Without it, work continues after the caller has gone away: a cancelled HTTP request still runs its database query.

## The rule
- Accept \`ctx context.Context\` as the first parameter of any function that does I/O or may block.
- Pass it down. Never replace it with \`context.Background()\` mid-request.

## Better
\`\`\`go
req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
rows, err := db.QueryContext(ctx, "SELECT ...")
func Fetch(ctx context.Context, id int) error
\`\`\`

## Try it yourself
Cancel the request context in a test and check that your handler stops promptly.`,
  },
  concurrency: {
    id: "concurrency",
    markdown: `# Concurrency: goroutine lifetimes

## Why it exists
Starting a goroutine is one keyword. Stopping it is your problem. A goroutine that blocks forever on a channel is a leak.

## The rule
Every goroutine needs an answer to two questions:
1. How does it stop?
2. Who waits for it?

## Better
\`\`\`go
g, ctx := errgroup.WithContext(ctx)
g.Go(func() error { return process(ctx, item) })
if err := g.Wait(); err != nil { return err }
\`\`\`

## Try it yourself
For each \`go\` statement in your PR, write one sentence saying when it exits. If you can't, that is the finding.`,
  },
  interfaces: {
    id: "interfaces",
    markdown: `# Interfaces: small and defined by the consumer

## The rule
- Prefer one to three methods.
- Define the interface where it is consumed, not next to the implementation.
- Don't introduce an interface until there is a second implementation or a test that needs a fake.

## Better
\`\`\`go
// in the package that sends the email
type userGetter interface {
    Get(ctx context.Context, id int64) (User, error)
}
\`\`\`

## Try it yourself
Pick a function that takes a large interface. List the methods it actually calls, then rewrite the parameter type to contain only those.`,
  },
  "sql-injection": {
    id: "sql-injection",
    markdown: `# SQL injection

## Why it exists
When user input is joined into SQL text, the database cannot tell your query from the attacker's.

## The rule
Never build SQL with string concatenation or formatting. Send values as parameters.

## Better
\`\`\`go
rows, err := db.QueryContext(ctx, "SELECT * FROM orders WHERE customer_id = $1", customerID)
\`\`\`

## Try it yourself
Search your code for \`Sprintf\` and \`+\` near SQL keywords.`,
  },
  srp: {
    id: "srp",
    markdown: `# Single Responsibility Principle

## Why it exists
A type should have one reason to change. When one type mixes concerns, an edit for one reason risks breaking the others.

## The rule
Describe the type in one sentence without the word "and". If you can't, split it.

## Try it yourself
List everything OrderService would change for: new invoice layout, new email provider, new order rule. Which of those should touch the same file?`,
  },
};
