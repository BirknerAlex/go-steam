# go-steam

A Go library for downloading Steam app content and Workshop items without the Steam client.

## Features

- Anonymous downloads for publicly accessible apps (no account required)
- Authenticated downloads for paid games and Workshop items
- Incremental updates — only downloads chunks that differ from what is already on disk
- Concurrent chunk downloads with configurable parallelism
- Supports VZip-ZSTD, VZip-LZMA, ZIP, and ZLib chunk formats
- Steam Guard support: TOTP authenticator (automatic) and email (via callback)
- Local cache for sessions, depot keys, and CDN tokens across runs

## Requirements

Go 1.26+

## Usage

### Library

```go
import steam "github.com/BirknerAlex/go-steam"

ctx := context.Background()

client, err := steam.New(ctx, steam.Config{
    // Omit Username/Password for anonymous access (free/public apps only).
    Username: "myaccount",
    Password: "mypassword",
    // SteamGuardCallback is called when Steam Guard is required.
    // Use steam.InteractiveSteamGuard to prompt stdin, or provide your own.
    SteamGuardCallback: steam.InteractiveSteamGuard,
    // CachePath defaults to ~/.cache/go-steam
})
if err != nil {
    log.Fatal(err)
}
defer client.Close()

// Download a Steam app.
ch, err := client.DownloadApp(ctx, steam.AppDownloadRequest{
    AppID:     1623730,   // Palworld
    OS:        "linux",
    TargetDir: "/opt/palworld",
})
if err != nil {
    log.Fatal(err)
}

for p := range ch {
    if p.Err != nil {
        log.Fatal(p.Err)
    }
    fmt.Printf("[%s] %d/%d chunks\n", p.Phase, p.DoneChunks, p.TotalChunks)
}
```

```go
// Download a Steam Workshop item (requires authenticated session).
ch, err := client.DownloadWorkshopItem(ctx, steam.WorkshopDownloadRequest{
    ItemID:    3625223587,
    TargetDir: "/tmp/workshop-item",
})
```

### CLI — app download

```
go run ./cmd/app-download \
  -app 1623730 \
  -os linux \
  -username myaccount \
  -password mypassword \
  -output /opt/palworld \
  -v
```

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `-app` | — | Steam App ID |
| `-depots` | all | Comma-separated depot IDs to restrict download |
| `-branch` | `public` | Branch name |
| `-branch-password` | — | Password for protected beta branches |
| `-os` | `linux` | OS filter: `linux`, `windows`, `macos`, or empty for all |
| `-lang` | all | Language filter (e.g. `english`) |
| `-username` | — | Steam account username (omit for anonymous) |
| `-password` | — | Steam account password |
| `-totp-secret` | — | Base64 TOTP shared secret for automatic Steam Guard |
| `-output` | `./output` | Destination directory |
| `-validate` | false | Verify on-disk files without re-downloading |
| `-cache` | `~/.cache/go-steam` | Cache directory |
| `-v` | false | Verbose logging |

### CLI — workshop download

```
go run ./cmd/workshop-download \
  -item 3625223587 \
  -username myaccount \
  -password mypassword \
  -output /tmp/workshop \
  -v
```

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `-item` | — | Workshop Published File ID |
| `-username` | — | Steam account username (required) |
| `-password` | — | Steam account password |
| `-totp-secret` | — | Base64 TOTP shared secret for Steam Guard |
| `-output` | `./output` | Destination directory |
| `-cache` | `~/.cache/go-steam` | Cache directory |
| `-v` | false | Verbose logging |

### CLI — Steam Guard code

```
go run ./cmd/guard -secret <base64-totp-secret>
```

## How it works

1. Connects to a Steam CM (Connection Manager) server over WebSocket (TLS)
2. Performs an AES encryption handshake using Steam's RSA public key
3. Authenticates anonymously or via the Steam Authentication v2 flow (RSA-encrypted password → optional Steam Guard → access token)
4. Caches the access token to disk so subsequent runs skip re-authentication
5. Fetches app info via the PICS protocol to discover depots and manifest GIDs
6. Obtains a manifest request code from the CM (required for modern CDN auth)
7. Downloads and decrypts the depot manifest (AES-CBC, then LZMA decompression)
8. Diffs the manifest against existing on-disk content using per-chunk SHA-1
9. Downloads only the missing/changed chunks from Steam CDN servers concurrently
10. For each chunk: decrypts (AES-256 ECB-IV+CBC), detects format (VZip-ZSTD / VZip-LZMA / ZIP / ZLib), decompresses, verifies SHA-1, and writes to disk

## Limitations

- Workshop downloads require an authenticated session with ownership of the parent app
- Email-based Steam Guard works interactively via `SteamGuardCallback`; TOTP is fully automatic with `-totp-secret`
- Anonymous sessions only work for apps that expose anonymous depot access
- Branch password support is wired up but not widely tested

## Acknowledgements

This project stands on a great deal of knowledge from the [SteamKit](https://github.com/SteamRE/SteamKit) project — the wire formats, the CM protocol, the manifest and depot-chunk layouts (VZip-LZMA, VZip-ZSTD, PKZip), and Steam's AES symmetric scheme were all learned from SteamKit's implementation and documentation. Several of our regression tests are direct ports of SteamKit's own test fixtures (real depot chunks, manifests, and PICS packets) so our decoders can be validated against the same ground truth. We are very grateful to the SteamKit maintainers and contributors for their excellent, long-running work.

## License

This project's source code is licensed under the [MIT License](LICENSE).

The binary test fixtures vendored under `internal/*/testdata/` are copied from SteamKit's test suite and remain under the **LGPL-2.1** license — they are used only by `go test` and are never compiled into the library. See [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) for full attribution.
