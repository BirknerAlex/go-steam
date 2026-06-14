package proto

// EMsg is the message type identifier used in Steam network packets.
// Values with the proto flag (bit 31 set) indicate protobuf-encoded bodies.
type EMsg uint32

const (
	// ProtoFlag marks a message as using protobuf encoding.
	ProtoFlag EMsg = 0x80000000

	// Encryption handshake (struct messages, not protobuf).
	EMsgChannelEncryptRequest  EMsg = 1303
	EMsgChannelEncryptResponse EMsg = 1304
	EMsgChannelEncryptResult   EMsg = 1305

	// Heartbeat.
	EMsgClientHeartBeat EMsg = 703

	// Hello — required first message after connecting (SteamKit2 CMClient.OnClientConnected).
	EMsgClientHello EMsg = 9805

	// Logon.
	EMsgClientLogon         EMsg = 5514
	EMsgClientLogOnResponse EMsg = 751

	// PICS — product info / access tokens.
	EMsgClientPICSProductInfoRequest  EMsg = 8903
	EMsgClientPICSProductInfoResponse EMsg = 8904
	EMsgClientPICSAccessTokenRequest  EMsg = 8905
	EMsgClientPICSAccessTokenResponse EMsg = 8906

	// CDN auth tokens.
	EMsgClientRequestCDNAuthToken  EMsg = 5546
	EMsgClientCDNAuthTokenResponse EMsg = 5547

	// Depot decryption key.
	EMsgClientGetDepotDecryptionKey         EMsg = 5438
	EMsgClientGetDepotDecryptionKeyResponse EMsg = 5439

	// Beta branch password check (unlocks password-protected branch manifests).
	EMsgClientCheckAppBetaPassword         EMsg = 5450
	EMsgClientCheckAppBetaPasswordResponse EMsg = 5451

	// Multi — batched responses.
	EMsgMulti EMsg = 1

	// Service method calls — used for modern Steam API (e.g. CDN auth token).
	// Values from SteamKit2 Resources/SteamLanguage/emsg.steamd.
	EMsgServiceMethod                        EMsg = 146
	EMsgServiceMethodResponse                EMsg = 147
	EMsgServiceMethodCallFromClient          EMsg = 151
	EMsgServiceMethodCallFromClientNonAuthed EMsg = 9804
)

// Base returns the EMsg without the proto flag.
func (e EMsg) Base() EMsg { return e &^ ProtoFlag }

// IsProto reports whether the proto flag is set.
func (e EMsg) IsProto() bool { return e&ProtoFlag != 0 }
