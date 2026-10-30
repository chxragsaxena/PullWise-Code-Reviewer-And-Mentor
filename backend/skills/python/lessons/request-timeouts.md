# Network calls need a deadline

## Why it exists
The `requests` library waits forever by default. One server that accepts the connection and
never answers can freeze a web worker, a cron job or a whole script, and nothing reports it.

## The rule
Every outbound call gets a timeout. Pass `timeout=` in seconds, or a `(connect, read)` pair.

## Common mistakes
```python
resp = requests.get("https://api.example.com/items")
```

## Better
```python
resp = requests.get("https://api.example.com/items", timeout=10)
resp.raise_for_status()
```

## Try it yourself
If the service you call hung for an hour, what would your code be doing during that hour?
