package steam

// Phase describes the current stage of a download operation.
type Phase int

const (
	PhaseResolving  Phase = iota // resolving app/workshop metadata
	PhaseManifest               // fetching and parsing manifests
	PhaseDiffing                // comparing on-disk state vs manifest
	PhaseDownloading            // downloading and writing chunks
	PhaseComplete               // download finished successfully
)

func (p Phase) String() string {
	switch p {
	case PhaseResolving:
		return "resolving"
	case PhaseManifest:
		return "manifest"
	case PhaseDiffing:
		return "diffing"
	case PhaseDownloading:
		return "downloading"
	case PhaseComplete:
		return "complete"
	default:
		return "unknown"
	}
}

// Progress is emitted on the channel returned by DownloadApp / DownloadWorkshopItem.
// All size fields are in bytes. The channel is closed when Phase == PhaseComplete
// or when an error occurs (Err != nil).
type Progress struct {
	Phase Phase

	// TotalBytes is the total uncompressed bytes to download (0 until diffing is done).
	TotalBytes int64
	// DoneBytes is the number of uncompressed bytes written so far.
	DoneBytes int64

	// TotalChunks is the total number of chunks to fetch (0 until diffing is done).
	TotalChunks int
	// DoneChunks is the number of chunks successfully written.
	DoneChunks int

	// CurrentFile is the file path currently being written (relative to TargetDir).
	CurrentFile string

	// Err is non-nil when the download failed. The progress channel is closed
	// immediately after emitting a Progress with Err set.
	Err error
}
