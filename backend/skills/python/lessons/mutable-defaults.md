# A default value is created once, not on every call

## Why it exists
Python evaluates default arguments when the function is defined. A list, dict or set used
as a default is therefore one shared object, and anything one call adds to it is still
there for the next call.

## The rule
Use `None` as the default and build the mutable value inside the function.

## Common mistakes
```python
def add_tag(tag, tags=[]):
    tags.append(tag)
    return tags

add_tag("a")  # ["a"]
add_tag("b")  # ["a", "b"]  <- surprise
```

## Better
```python
def add_tag(tag, tags=None):
    tags = [] if tags is None else tags
    tags.append(tag)
    return tags
```

## Try it yourself
Call your function twice in a row without the argument. Do the results leak into each other?
