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

### What is the connector

Traefik doesn't like package `unsafe` (which has been widely used in many dependencies), so we have to split a dedicated `connector` to call them, contact with our plugin through `rpc` .

### Why this repo name

Lint doesn't like `GatewayFlowController_test` because of mixed camelCase and snake_case.

Traefik doesn't like `plugin` because of unmatched package name.

So in order to satisfy both of them, we have to use this long piece of word.
