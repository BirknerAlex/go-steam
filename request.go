package steam

// AppDownloadRequest describes a Steam app content download.
type AppDownloadRequest struct {
	// AppID is the Steam application ID to download.
	AppID uint32

	// DepotIDs optionally restricts which depots to download. When nil, all
	// depots eligible for the current platform and language are downloaded.
	DepotIDs []uint32

	// Branch is the beta branch name. Leave empty or "public" for the default branch.
	Branch string

	// BranchPassword is required for password-protected beta branches.
	BranchPassword string

	// OS filters depots by target OS: "windows", "linux", or "macos". Leave
	// empty to auto-detect the OS this process is running on (via
	// runtime.GOOS). Depots without OS metadata (shared/common depots) are
	// always included regardless of this filter.
	//
	// DownloadApp rejects the request before downloading anything if OS is
	// set to a value other than the three above, or if it's empty and
	// runtime.GOOS isn't one Steam supports (e.g. freebsd) -- there is no
	// "download every platform" fallback.
	OS string

	// Language filters language depots. Leave empty for no language filtering.
	Language string

	// TargetDir is the directory where content will be written.
	// Existing files are diffed against the manifest; only changed chunks are downloaded.
	TargetDir string

	// ValidateOnly skips writing files — only verifies on-disk chunks against the manifest.
	ValidateOnly bool
}

// WorkshopDownloadRequest describes a Steam Workshop item download.
type WorkshopDownloadRequest struct {
	// ItemID is the PublishedFileID (workshop item ID).
	ItemID uint64

	// TargetDir is the directory where workshop files will be written.
	TargetDir string
}
