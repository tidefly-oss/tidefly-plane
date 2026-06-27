package system

// VersionInfo is the exported type for version check results.
// Used by the dashboard overview to include version data without a separate API call.
type VersionInfo = versionInfo

// GetCachedVersion returns the current cached version info, or nil if not yet populated.
// The cache is populated by StartVersionRefresh on app startup.
func GetCachedVersion() *VersionInfo {
	return globalVersionCache.get()
}
