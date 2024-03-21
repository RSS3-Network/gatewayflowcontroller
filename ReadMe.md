# RSS3 Gateway Flow Controller

## About

This is a Traefik plugin, which controls the inbound traffic, useful for billing related requirements.

We use this with the [Gateway](https://github.com/RSS3-Network/Gateway).

Or you can also develop your own billing backend.

## Questions

### Why vendor dependencies

According to [traefik plugin demo's readme](https://github.com/traefik/plugindemo/blob/8a77aea29f9038903ab44059e2aa42a37ff52752/readme.md?plain=1#L27-L28),

> Plugin dependencies must be [vendored](https://golang.org/ref/mod#vendoring) for each plugin.
> Vendored packages should be included in the plugin's GitHub repository. ([Go modules](https://blog.golang.org/using-go-modules) are not supported.)

So we have to push the vendor directory.

### What if my test fails

The test sometimes fails for unknown reasons (maybe connective / timing issues between this plugin and other components?). If you think your code is fine, just re-run the test a few more times, it will eventually be Success.

If it keeps failing... Maybe let's take a closer look at those new code?

I mean, only the **Test**. If the Lint fails, it fails.

### What is the connector

Traefik doesn't like package `unsafe` (which has been widely used in many dependencies), so we have to split a dedicated `connector` to call them, contact with our plugin through `rpc` .
