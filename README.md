# Socquette

Socquette is a small authenticated **SOCKS5** (TCP and UDP) and **HTTP** proxy in a single binary, with a desktop
window to configure it. It is built to run on a Windows PC at home, so you can route your traffic through your home
connection while you are away. Linux works too.

- SOCKS5 with `CONNECT` and `UDP ASSOCIATE`, username/password authentication (RFC 1928 / RFC 1929).
- HTTP proxy with `CONNECT` tunnels (HTTPS) and plain HTTP forwarding, Basic authentication.
- Users with a password and, for each, the right to use the HTTP proxy, the SOCKS proxy or both. Optional anonymous
  access per proxy (off by default), and trusted networks (IPs or CIDR subnets) accepted without a password.
- Configuration in a `socquette.toml` file, applied on the fly by the running server.
- A configuration window (French, English, Spanish; light, dark or automatic theme) that also installs Socquette as a
  Windows service and starts or stops it.

## 🤖 AI Usage Disclaimer

This project was built entirely via "vibe coding" using **Claude Opus 5.5**. For me this project is both motivated by
a utilitarian point of view and an educational one: I wanted to test how I can write a complete Go app without much
knowledge of Go and its ecosystem.

As an experienced software developer with a very limited background in Go, I relied on AI assistance for the complete
codebase, documentation included, except some paragraphs of this README file.

While the project is functional and tested for its intended use case (i.e. my own personal usage), please keep in mind:
- **Code Quality:** Parts of the implementation might not follow idiomatic Go patterns.
- **Security & Performance:** The code has not been audited by an experienced Go engineer.
- **Contributions:** PRs, refactoring, and idiomatic Go improvements are extremely welcome!

For those reasons, I won't provide any pre-compiled binary. Build and use it at your own risk.

## Download

Get `socquette-<version>-windows-x86_64.exe` (or the Linux build) from the releases page of the repository.

## Quick start

1. Put `socquette.exe` in a folder of its own, e.g. `C:\Socquette`, and double-click it: the configuration window
   opens. It edits `socquette.toml` in the current folder (or in the folder named by the `SOCQUETTE_CONF` environment
   variable).
2. Enable the proxies you want, add a user, **Save**.
3. Click **Install service** (Windows asks for administrator rights): Socquette now starts with Windows.
4. On your router, forward the proxy ports to this PC (for SOCKS, forward the port in **TCP and UDP**).

Then point your applications at it, for instance:

```bash
curl --socks5-hostname alice:password@my-home.example.org:1080 https://example.com
```

See the [user guide](docs/user-guide.md) for the details, including how to put the HTTP proxy behind a TLS reverse
proxy.

## Security

Proxy credentials travel in clear text (that's how SOCKS5 and HTTP proxy authentication work): use passwords dedicated
to Socquette. The traffic you send through it (HTTPS sites, for instance) stays encrypted end to end.

## Documentation

- [User guide](docs/user-guide.md)
- [Configuration file](docs/configuration.md)
- [Architecture](docs/architecture.md)
- [Development](docs/development.md)

## License

[EUPL-1.2](LICENSE)
