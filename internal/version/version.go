// Package version holds the Kuvryn Sync build version.
package version

// Version is the Kuvryn Sync release the binary was built from. Builds set it with
//
//	-ldflags "-X github.com/azrtydxb/kuvryn-sync/internal/version.Version=v0.3.0"
//
// and it is "dev" otherwise.
var Version = "dev"
