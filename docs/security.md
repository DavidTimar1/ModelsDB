# Security Policy

By default ModelsDB runs entirely on your own machine. It serves a web page on `localhost`, keeps everything it knows in one local SQLite file, and reaches the network in only three places: OpenRouter, for the public model catalog; HuggingFace, when you ask it to read a model's repository; and GitHub, to check whether a newer release exists. There are no accounts, no telemetry, and no server of its own.

It has no authentication. If you bind an address other than loopback, anyone who can reach that address can read and change everything, so use a private address; the app refuses a public or wildcard bind unless you pass `--allow-public`. A new version is trusted only when the release's checksum file is signed with the key compiled into the app that is updating.

## What the server checks

- **Host.** Every request must name a host the server is known by: `localhost`, a loopback address, an address it bound, or the host of an `allowed_origins` entry in `config.jsonc`. Anything else gets 403, which defeats a web page that rebinds its own name to this machine.
- **Writes** from a browser page on another origin are refused.
- **Bodies** are capped at 16 MiB, and the server has header, read, write and idle timeouts so a silent client cannot hold a connection open.
- **Framing and sniffing.** Every answer carries `X-Frame-Options: DENY`, a `frame-ancestors 'none'` content security policy and `X-Content-Type-Options: nosniff`, so no other site can frame the page.
- **Outbound requests** all carry one plain browser User-Agent and nothing that names this app or its user.
- **Backups** are written only into the folder saved in the settings, or the folder you name to `modelsdb backup`.

## Reporting a problem

Found a security issue, or something that just looks off? Please tell me.

You can reach me through any of the channels listed on my GitHub profile: https://github.com/DavidTimar1. If the issue is sensitive, please use a private channel rather than posting it in the open.

This is a hobby project, so I can't promise a fixed turnaround. I do read every report and take real issues seriously.

## Supported versions

Fixes go into the latest release. Please check against the newest binary before reporting.
