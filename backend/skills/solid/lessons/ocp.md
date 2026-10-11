# Open/Closed Principle

## Why it exists
Code should be open to extension but closed to modification: adding a new
case shouldn't require editing and re-testing logic that already works for
every existing case. Every edit to working code is a chance to break it.

## The rule
If adding a new kind of something means adding a new branch to an existing
`if`/`switch` that already has several, the design is closed to extension.
Prefer a shape where the new case is purely additive — a new type, a new
registered handler — rather than an edit to shared logic.

## Common mistakes
```go
func Discount(order Order) float64 {
    switch order.CustomerTier {
    case "bronze":
        return order.Total * 0.0
    case "silver":
        return order.Total * 0.05
    case "gold":
        return order.Total * 0.10
    // every new tier means editing this function and re-testing all the others
    }
    return 0
}
```

## Better
```go
type DiscountRule interface {
    Discount(order Order) float64
}

func ApplyDiscount(order Order, rule DiscountRule) float64 {
    return rule.Discount(order)
}
```
A new tier is a new `DiscountRule` implementation. `ApplyDiscount` — and
every existing tier's logic — is never touched again.

## When this doesn't apply
A short `switch` with two or three cases that rarely change is not a
violation; introducing an interface for it is often the over-engineering
this principle warns against on the other side. The concern is a `switch`
that keeps growing and that different people keep editing for unrelated
reasons.

## Try it yourself
Find a conditional in your PR that branches on a type, category or kind.
Ask: when the next kind arrives, what existing code has to change? If the
answer is "none, just add a case somewhere new," it's already open for
extension.
