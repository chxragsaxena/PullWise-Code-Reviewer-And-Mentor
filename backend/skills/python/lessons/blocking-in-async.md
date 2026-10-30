# One blocking call stops the whole event loop

## Why it exists
An `async def` function shares a single thread with every other coroutine. A blocking call
inside it, such as `time.sleep`, `requests.get` or a synchronous database driver, freezes all
of them until it returns.

## The rule
Inside `async def`, use awaitable libraries (`asyncio.sleep`, `httpx.AsyncClient`, async
database drivers), or move unavoidable blocking work to a thread with `asyncio.to_thread`.

## Common mistakes
```python
async def fetch(url):
    time.sleep(1)
    return requests.get(url, timeout=5).text
```

## Better
```python
async def fetch(url):
    await asyncio.sleep(1)
    async with httpx.AsyncClient(timeout=5) as client:
        return (await client.get(url)).text
```

## Try it yourself
While this function runs, can any other request in the same process make progress?
