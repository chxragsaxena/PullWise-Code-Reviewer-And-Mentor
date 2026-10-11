# Name resources in the URL; let the HTTP method be the verb

## Why it exists
A URL like `/getUser?id=5` duplicates the verb that `GET` already states,
and it has to grow a new path for every new action (`/updateUser`,
`/deleteUser`, `/listUsers`...). A URL that names the resource instead scales
naturally: the same path works for read, update and delete, distinguished
only by method.

## The rule
- URLs name resources (nouns), usually plural: `/users`, `/orders`.
- A specific resource is a path segment: `/users/{id}`.
- A related collection nests under it: `/users/{id}/orders`.
- The action comes from the HTTP method, not the path.

## Common mistakes
```
GET  /getUser?id=5
POST /createOrder
POST /updateOrderStatus?id=5&status=shipped
```

## Better
```
GET   /users/5
POST  /orders
PATCH /orders/5          { "status": "shipped" }
```

## When a verb-shaped path is fine
Some actions genuinely aren't CRUD on a resource (`/orders/5/cancel`,
`/reports/export`). That's acceptable when the action isn't really "update a
field" — just don't reach for it as a default instead of resource shape.

## Try it yourself
For an endpoint in your PR, check: does the path contain a verb that the
HTTP method already expresses? If so, what would the resource-and-method
version of the same path look like?
