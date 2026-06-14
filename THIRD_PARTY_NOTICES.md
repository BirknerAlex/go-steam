# Third-Party Notices

This project (`go-steam`) is licensed under the [MIT License](LICENSE). It is an
independent Go reimplementation of parts of the Steam content-delivery protocol.

The protocol knowledge it is built on — wire formats, message layouts, the depot
manifest and chunk container formats, and Steam's symmetric crypto scheme — was
learned from the **SteamKit** project. Facts and interoperability information are
not themselves copyrightable, and an independent reimplementation of a protocol
is not a derivative work, so the project's own source code is MIT-licensed.

However, this repository also **vendors a number of binary test-fixture files
copied verbatim from SteamKit's own test suite**. Those files are the
copyrighted material of the SteamKit authors and are redistributed here under
their original license. They are used only by `go test`; they are never compiled
into the library or any binary built from it.

## SteamKit

- **Project:** SteamKit (SteamKit2)
- **Source:** https://github.com/SteamRE/SteamKit
- **Copyright:** © The SteamKit contributors
- **License:** GNU Lesser General Public License, version 2.1 (LGPL-2.1)
  - https://github.com/SteamRE/SteamKit/blob/master/LICENSE

### Vendored files (LGPL-2.1)

The following files were copied from `SteamKit2/Tests/Files` and
`SteamKit2/Tests/Packets` and remain under LGPL-2.1:

```
internal/cdn/testdata/depot_440_chunk_bac8e2657470b2eb70d6ddcd6c07004be8738697.bin
internal/cdn/testdata/depot_232250_chunk_7b8567d9b3c09295cdbf4978c32b348d8e76c750.bin
internal/cdn/testdata/depot_3441461_chunk_9e72678e305540630a665b93e1463bc3983eb55a.bin
internal/cdn/testdata/depot_440_1118032470228587934.manifest
internal/cdn/testdata/depot_440_1118032470228587934_decrypted.manifest
internal/cdn/testdata/depot_440_1118032470228587934_v4.manifest
internal/cm/testdata/002_in_8904_k_EMsgClientPICSProductInfoResponse_app480.bin
internal/proto/testdata/001_in_8904_k_EMsgClientPICSProductInfoResponse_app480_metadata.bin
internal/proto/testdata/002_in_8904_k_EMsgClientPICSProductInfoResponse_app480.bin
internal/proto/testdata/003_in_8904_k_EMsgClientPICSProductInfoResponse_sub0.bin
```

These fixtures additionally contain real Steam content (e.g. depot chunks and
PICS responses for Valve's Team Fortress 2 / Spacewar apps); that underlying
content is the property of Valve Corporation and is reproduced here, as in
SteamKit, solely for interoperability testing.

The Go test code that exercises these fixtures
(`internal/*/steamkit_*_test.go`) is original work under the MIT License, though
its structure and the expected values it asserts were derived from SteamKit's
corresponding tests (`DepotChunkFacts.cs`, `DepotManifestFacts.cs`,
`PacketFacts.cs`).
