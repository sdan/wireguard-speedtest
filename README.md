# wireguard-speedtest

Print latency analysis of Wireguard config files. This is useful if you use a consumer VPN service like Mullvad and/or have a bunch of Wireguard configurations that you want to know the latency to.

## Install & run

`go mod install`

`go run main.go`

## Example Output

```
Top 10 fastest peers:
1. (us111-wireguard.conf) 8.374ms
2. (at9-wireguard.conf) 98.325667ms
3. (at6-wireguard.conf) 98.662666ms

```
It will also save stats directly to `stats.txt`
## Prereqs
Have your wireguard configuration files in a `config/` dir as such:

```
wg-netcheck
│   README.md
│   main.go
│
└───config
│   │   de13-wireguard.conf
│   │   ch-zrh-wg-402.conf
│   │   ca23-wireguard.conf
│   │   gb20-wireguard.conf
│   │   pl2-wireguard.conf
│   │   us-dal-wg-102.conf
│   │   us-sjc-wg-104.conf
```

Make sure you delete any irrelevant dot files such as .DS_Store in the config dir!

### Limitations
Works for IPv4 addresses only, IPv6 supported soon
