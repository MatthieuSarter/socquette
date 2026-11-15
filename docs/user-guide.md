# User guide

## The configuration window

Run `socquette` without arguments (double-click on Windows). The window edits `socquette.toml` (see
[configuration](configuration.md)); nothing is written until you click **Save**.

- **Server**: shows whether the server runs, as a service or as a background process.
  - **Start / Stop**: when the service is installed, starts or stops the service (Windows asks for administrator
    rights). Otherwise, starts a background server that keeps running after the window is closed, or stops it.
  - **Install service / Uninstall service**: installs Socquette as a Windows service that starts with Windows (and
    starts it right away), or removes it. Installing stops a background server first. The service reads the
    configuration of the folder the window was using.
  - **Configure firewall**: allows the ports of the enabled proxies (HTTP in TCP, SOCKS5 in TCP and UDP) through the
    Windows firewall, in rules named "Socquette" limited to this program (Windows asks for administrator rights). Each
    click replaces the previous rules, so click it again after changing a port. Pending changes are saved first. On
    Linux it does the same with `ufw` (rules commented "Socquette"); without ufw, open the ports by hand.
- **HTTP proxy / SOCKS5 proxy**: enable each proxy, choose the address to listen on (the list suggests all interfaces,
  loopback and this PC's addresses; you can type another) and the port. **Anonymous access** lets anyone use the proxy
  without a password: the window asks for confirmation, since an open proxy on the Internet is quickly found and
  abused.
- **Public address (UDP)**: see [UDP behind a router](#udp-behind-a-router).
- **Users**: add users with a password (typed twice), choose who may use which proxy, change a password, delete a
  user. Passwords are stored hashed and can't be displayed again.
- **Language** and **Theme** (automatic follows Windows) are remembered.

## Running without the window

```bash
socquette --serve
```

runs the server in the console (logs on screen and in `socquette.log`) until Ctrl+C. `socquette --version` prints the
version.

On Linux, the window installs a systemd unit; it needs to run as root for that (`sudo socquette`).

## Reaching it from the Internet

1. On your router, forward the proxy ports to the PC running Socquette. For SOCKS5, forward the port in **TCP and
   UDP** (the UDP relay uses the same port number).
2. Allow the ports in the firewall: **Configure firewall** in the window.
3. If your public IP changes, use a dynamic DNS name to reach it.

### UDP behind a router

A SOCKS5 client asks for a UDP relay and is told where to send its datagrams. Behind a router, Socquette only knows
its local address, so set **Public address (UDP)** to your public IP or dynamic DNS name. Left empty, Socquette
announces `0.0.0.0`, which most clients read as "the address I'm already connected to" — try that first.

## Client examples

```bash
curl --socks5-hostname alice:password@my-home.example.org:1080 https://example.com
curl -x http://alice:password@my-home.example.org:8080 https://example.com
```

Browsers (Firefox, Chrome) can't authenticate to a SOCKS5 proxy: point them at the **HTTP proxy** instead (Firefox:
Settings → Network settings → Manual proxy configuration → HTTP proxy, and tick "Also use this proxy for HTTPS"); they
prompt for the user name and password. SOCKS5 with authentication needs a client that supports it: curl, proxy
extensions, or system-wide tunnel clients (tun2socks and the like), which also carry UDP.

## Credentials in clear text

SOCKS5 and HTTP proxy authentication send the user name and password unencrypted: anyone on the same network as you
(hotel Wi-Fi...) can read them. Use passwords dedicated to Socquette. The content of encrypted connections (HTTPS
sites...) stays protected.

### HTTP proxy behind a TLS reverse proxy

To encrypt the HTTP proxy credentials, put a TLS-terminating **TCP stream** in front of it and use it as an "HTTPS
proxy". With Nginx Proxy Manager, that's a **Stream** (not a Proxy Host: nginx's HTTP module can't relay `CONNECT`),
forwarding for instance port 8443 to Socquette's HTTP port, with a certificate. With plain nginx:

```nginx
stream {
    server {
        listen 8443 ssl;
        ssl_certificate     /etc/ssl/proxy.crt;
        ssl_certificate_key /etc/ssl/proxy.key;
        proxy_pass 192.168.1.10:8080;
    }
}
```

Clients then connect over TLS: `curl --proxy https://alice:password@my-home.example.org:8443 https://example.com`, or a
PAC file returning `HTTPS my-home.example.org:8443` in Firefox or Chrome. Socquette then logs the reverse proxy's
address as the client. This doesn't apply to SOCKS5 (clients don't speak SOCKS over TLS) nor to UDP.
