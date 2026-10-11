# Single Responsibility Principle

## Why it exists
A type should have one reason to change. When a type mixes concerns, a
change made for one reason — a new email template, a different invoice
format — risks breaking behavior that has nothing to do with that change,
because it all lives in the same file, the same struct, the same tests.

## The rule
Describe the type in one sentence without the word "and". If you can't,
it's doing more than one job.

## Common mistakes
```go
type OrderService struct{ db *sql.DB; mailer Mailer }

func (s *OrderService) Create(o Order) error {
    // 1. validate and save the order
    if err := s.db.Save(o); err != nil {
        return err
    }
    // 2. render and send a confirmation email
    body := fmt.Sprintf("Thanks for order %d", o.ID)
    return s.mailer.Send(o.CustomerEmail, body)
    // 3. (elsewhere in the same type) generate invoice PDFs, apply discount
    //    rules, sync to the warehouse system...
}
```
`OrderService` now changes for order-validation reasons, email-wording
reasons, and invoice-format reasons. A bug fix in one risks a regression in
another, and the tests for all three are entangled.

## Better
```go
type OrderService struct {
    db        OrderStore
    notifier  OrderNotifier // sends the confirmation; OrderService doesn't know how
}

func (s *OrderService) Create(o Order) error {
    if err := s.db.Save(o); err != nil {
        return err
    }
    return s.notifier.NotifyCreated(o)
}
```
Each concern — persistence, notification, invoicing — gets its own type with
its own reason to change.

## Try it yourself
Pick a type in your PR and list everything it would need to change for:
a new business rule, a new output format, a new delivery channel. If that
list names more than one kind of change, it's more than one responsibility.
