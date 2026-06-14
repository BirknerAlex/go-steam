package proto

import (
	"fmt"

	"google.golang.org/protobuf/encoding/protowire"
)

// ContentManifest is the outer wrapper returned by the Steam CDN (zip entry).
// Field 1 holds raw bytes for the (possibly AES-CBC-encrypted) ContentManifestPayload.
// Field 2 holds raw bytes for the ContentManifestMetadata.
type ContentManifest struct {
	PayloadBytes  []byte // field 1
	MetadataBytes []byte // field 2
}

func (m *ContentManifest) Unmarshal(data []byte) error {
	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			return fmt.Errorf("proto: bad tag in ContentManifest")
		}
		data = data[n:]
		switch {
		case num == 1 && typ == protowire.BytesType:
			v, n := protowire.ConsumeBytes(data)
			if n < 0 {
				return fmt.Errorf("proto: bad bytes in ContentManifest.payload")
			}
			m.PayloadBytes = append([]byte(nil), v...)
			data = data[n:]
		case num == 2 && typ == protowire.BytesType:
			v, n := protowire.ConsumeBytes(data)
			if n < 0 {
				return fmt.Errorf("proto: bad bytes in ContentManifest.metadata")
			}
			m.MetadataBytes = append([]byte(nil), v...)
			data = data[n:]
		default:
			n := protowire.ConsumeFieldValue(num, typ, data)
			if n < 0 {
				return fmt.Errorf("proto: unknown field %d in ContentManifest", num)
			}
			data = data[n:]
		}
	}
	return nil
}

// ContentManifestPayload is the top-level structure of a decrypted depot manifest.
// Field numbers from content_manifest.proto in SteamKit2.
type ContentManifestPayload struct {
	Mappings []ContentManifestFile // field 1
}

// ContentManifestFile describes one file in a depot manifest.
type ContentManifestFile struct {
	Filename         string                   // field 1
	Size             uint64                   // field 2
	Flags            uint32                   // field 3
	ShaFilename      []byte                   // field 4 — SHA1 of filename
	ShaContent       []byte                   // field 5 — SHA1 of file content
	Chunks           []ContentManifestChunk   // field 6
	LinktargetPath   string                   // field 7
}

// ContentManifestChunk describes one chunk within a file.
type ContentManifestChunk struct {
	Sha              []byte // field 1 — SHA1 / chunk ID used in CDN URL
	Crc              uint32 // field 2 — Adler-32 of compressed data
	Offset           uint64 // field 3 — byte offset within the file
	CbOriginal       uint32 // field 4 — uncompressed size
	CbCompressed     uint32 // field 5 — compressed size on CDN
}

func unmarshalChunk(data []byte) (ContentManifestChunk, error) {
	var c ContentManifestChunk
	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			return c, fmt.Errorf("proto: bad tag in chunk")
		}
		data = data[n:]
		switch {
		case num == 1 && typ == protowire.BytesType:
			v, n := protowire.ConsumeBytes(data)
			if n < 0 {
				return c, fmt.Errorf("proto: bad bytes")
			}
			c.Sha = append([]byte(nil), v...)
			data = data[n:]
		case num == 2 && typ == protowire.Fixed32Type:
			v, n := protowire.ConsumeFixed32(data)
			if n < 0 {
				return c, fmt.Errorf("proto: bad fixed32")
			}
			c.Crc = v
			data = data[n:]
		case num == 3 && typ == protowire.VarintType:
			v, n := protowire.ConsumeVarint(data)
			if n < 0 {
				return c, fmt.Errorf("proto: bad varint")
			}
			c.Offset = v
			data = data[n:]
		case num == 4 && typ == protowire.VarintType:
			v, n := protowire.ConsumeVarint(data)
			if n < 0 {
				return c, fmt.Errorf("proto: bad varint")
			}
			c.CbOriginal = uint32(v)
			data = data[n:]
		case num == 5 && typ == protowire.VarintType:
			v, n := protowire.ConsumeVarint(data)
			if n < 0 {
				return c, fmt.Errorf("proto: bad varint")
			}
			c.CbCompressed = uint32(v)
			data = data[n:]
		default:
			n := protowire.ConsumeFieldValue(num, typ, data)
			if n < 0 {
				return c, fmt.Errorf("proto: unknown field %d", num)
			}
			data = data[n:]
		}
	}
	return c, nil
}

func unmarshalFile(data []byte) (ContentManifestFile, error) {
	var f ContentManifestFile
	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			return f, fmt.Errorf("proto: bad tag in file")
		}
		data = data[n:]
		switch {
		case num == 1 && typ == protowire.BytesType:
			v, n := protowire.ConsumeString(data)
			if n < 0 {
				return f, fmt.Errorf("proto: bad string")
			}
			f.Filename = v
			data = data[n:]
		case num == 2 && typ == protowire.VarintType:
			v, n := protowire.ConsumeVarint(data)
			if n < 0 {
				return f, fmt.Errorf("proto: bad varint")
			}
			f.Size = v
			data = data[n:]
		case num == 3 && typ == protowire.VarintType:
			v, n := protowire.ConsumeVarint(data)
			if n < 0 {
				return f, fmt.Errorf("proto: bad varint")
			}
			f.Flags = uint32(v)
			data = data[n:]
		case num == 4 && typ == protowire.BytesType:
			v, n := protowire.ConsumeBytes(data)
			if n < 0 {
				return f, fmt.Errorf("proto: bad bytes")
			}
			f.ShaFilename = append([]byte(nil), v...)
			data = data[n:]
		case num == 5 && typ == protowire.BytesType:
			v, n := protowire.ConsumeBytes(data)
			if n < 0 {
				return f, fmt.Errorf("proto: bad bytes")
			}
			f.ShaContent = append([]byte(nil), v...)
			data = data[n:]
		case num == 6 && typ == protowire.BytesType:
			v, n := protowire.ConsumeBytes(data)
			if n < 0 {
				return f, fmt.Errorf("proto: bad bytes")
			}
			chunk, err := unmarshalChunk(v)
			if err != nil {
				return f, err
			}
			f.Chunks = append(f.Chunks, chunk)
			data = data[n:]
		case num == 7 && typ == protowire.BytesType:
			v, n := protowire.ConsumeString(data)
			if n < 0 {
				return f, fmt.Errorf("proto: bad string")
			}
			f.LinktargetPath = v
			data = data[n:]
		default:
			n := protowire.ConsumeFieldValue(num, typ, data)
			if n < 0 {
				return f, fmt.Errorf("proto: unknown field %d", num)
			}
			data = data[n:]
		}
	}
	return f, nil
}

func (m *ContentManifestPayload) Unmarshal(data []byte) error {
	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			return fmt.Errorf("proto: bad tag in ContentManifestPayload")
		}
		data = data[n:]
		switch {
		case num == 1 && typ == protowire.BytesType:
			v, n := protowire.ConsumeBytes(data)
			if n < 0 {
				return fmt.Errorf("proto: bad bytes")
			}
			f, err := unmarshalFile(v)
			if err != nil {
				return err
			}
			m.Mappings = append(m.Mappings, f)
			data = data[n:]
		default:
			n := protowire.ConsumeFieldValue(num, typ, data)
			if n < 0 {
				return fmt.Errorf("proto: unknown field %d", num)
			}
			data = data[n:]
		}
	}
	return nil
}

// ContentManifestMetadata describes the depot and build info for a manifest.
type ContentManifestMetadata struct {
	DepotID         uint32 // field 1
	GIDManifest     uint64 // field 2
	CreationTime    uint32 // field 3
	FilenamesEncrypted bool // field 4
	CbDiskOriginal  uint64 // field 5
	CbDiskCompressed uint64 // field 6
	UniqueChunks    uint32 // field 7
	CrcEncrypted    uint32 // field 8
	CrcClear        uint32 // field 9
}

func (m *ContentManifestMetadata) Unmarshal(data []byte) error {
	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			return fmt.Errorf("proto: bad tag in ContentManifestMetadata")
		}
		data = data[n:]
		switch {
		case num == 1 && typ == protowire.VarintType:
			v, n := protowire.ConsumeVarint(data)
			if n < 0 {
				return fmt.Errorf("proto: bad varint")
			}
			m.DepotID = uint32(v)
			data = data[n:]
		case num == 2 && typ == protowire.VarintType:
			v, n := protowire.ConsumeVarint(data)
			if n < 0 {
				return fmt.Errorf("proto: bad varint")
			}
			m.GIDManifest = v
			data = data[n:]
		case num == 3 && typ == protowire.VarintType:
			v, n := protowire.ConsumeVarint(data)
			if n < 0 {
				return fmt.Errorf("proto: bad varint")
			}
			m.CreationTime = uint32(v)
			data = data[n:]
		case num == 4 && typ == protowire.VarintType:
			v, n := protowire.ConsumeVarint(data)
			if n < 0 {
				return fmt.Errorf("proto: bad varint")
			}
			m.FilenamesEncrypted = v != 0
			data = data[n:]
		default:
			n := protowire.ConsumeFieldValue(num, typ, data)
			if n < 0 {
				return fmt.Errorf("proto: unknown field %d", num)
			}
			data = data[n:]
		}
	}
	return nil
}
