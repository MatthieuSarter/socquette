# Configuration

## Files

Socquette uses one directory: the one named by the `SOCQUETTE_CONF` environment variable when it is set, else the
current directory. It holds:

| File             | Written by                         | Content                                         |
|------------------|------------------------------------|-------------------------------------------------|
| `socquette.toml` | the configuration window (or you)  | the configuration below                         |
| `socquette.log`  | the server, service actions        | connections, authentication failures, reloads   |
| `socquette.pid`  | the window, for a background server | PID of the server started without the service |

The Windows service and the systemd unit get `SOCQUETTE_CONF` set to the directory the window was using when it
installed them, so they read the same file.

The running server checks the file every 2 seconds and applies changes on the fly: new users and rights apply to new
connections, listener changes rebind the proxy. Connections already open are not cut (except UDP relaying of a SOCKS
proxy whose address changes). An invalid file is reported in `socquette.log` and ignored: the server keeps the last
valid configuration. A missing file means both proxies are disabled.

## `socquette.toml`

```toml
[http]
enabled = true
listen = "0.0.0.0"
port = 8080
anonymous = false

[socks]
enabled = true
listen = "0.0.0.0"
port = 1080
anonymous = false
advertise_address = "my-home.example.org"

[[users]]
name = "alice"
password_hash = "$2a$10$..."
http = true
socks = true

[[networks]]
name = "lan"
addresses = ["192.168.1.0/24", "fd00::/8", "203.0.113.7"]
http = true
socks = true
```

| Key                       | Default   | Meaning                                                                             |
|---------------------------|-----------|-------------------------------------------------------------------------------------|
| `http.enabled`            | `false`   | Run the HTTP proxy.                                                                 |
| `http.listen`             | `0.0.0.0` | IP address to listen on: `0.0.0.0` (all IPv4), `::` (all), `127.0.0.1`, an interface IP. |
| `http.port`               | `8080`    | TCP port, 1–65535.                                                                  |
| `http.anonymous`          | `false`   | Also accept clients that send no credentials. Wrong credentials are still refused. |
| `socks.enabled`           | `false`   | Run the SOCKS5 proxy.                                                               |
| `socks.listen`            | `0.0.0.0` | As for HTTP.                                                                        |
| `socks.port`              | `1080`    | TCP port, and UDP port of the UDP relay (same number).                              |
| `socks.anonymous`         | `false`   | As for HTTP.                                                                        |
| `socks.advertise_address` | `""`      | IP or host name announced to UDP clients (your public address); empty: `0.0.0.0`. |
| `users[].name`            |           | User name, unique, at most 255 bytes.                                               |
| `users[].password_hash`   |           | bcrypt hash of the password (the window computes it).                              |
| `users[].http`            | `false`   | The user may use the HTTP proxy.                                                    |
| `users[].socks`           | `false`   | The user may use the SOCKS proxy.                                                   |
| `networks[].name`         |           | Name of a trusted network, unique; shown in the log's `user` column.              |
| `networks[].addresses`    |           | At least one IP or CIDR subnet, IPv4 or IPv6 (`192.168.1.0/24`, `fd00::/8`); a bare IP is a `/32` or `/128`. |
| `networks[].http`         | `false`   | Clients from these addresses may use the HTTP proxy without credentials.           |
| `networks[].socks`        | `false`   | Clients from these addresses may use the SOCKS proxy without credentials.          |

A client that sends no credentials is accepted when its source address is in a trusted network that has the right to
use the proxy (the first such network, in file order, names it in the log), else when the proxy is anonymous. A client
that sends credentials is checked on them alone: wrong credentials are refused even from a trusted network. On a
proxy listening on `::`, IPv4 clients (seen as `::ffff:a.b.c.d`) match the IPv4 entries. The source address is the TCP
peer's: behind a reverse proxy (see the [user guide](user-guide.md#http-proxy-behind-a-tls-reverse-proxy)), it is the
reverse proxy's, and `X-Forwarded-For` is not trusted.

The HTTP and SOCKS proxies can't both be enabled on the same address and port. A host name in `advertise_address`
(e.g. a dynamic DNS name) is resolved at every UDP association, so a changing home IP is followed.

To compute a password hash by hand, any bcrypt tool works, e.g. `htpasswd -nbB alice password` (keep the part after
`alice:`).

## Configuration window preferences

The window's language and theme are not in `socquette.toml`: they are stored by the window itself (Fyne preferences,
under `%APPDATA%\fyne\io.github.matthieusarter.socquette` on Windows).
