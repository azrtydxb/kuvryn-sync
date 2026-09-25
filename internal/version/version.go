// Package version holds the Solder build version.
package version

// Version is the Solder release the binary was built from. Builds set it with
//
//	-ldflags "-X github.com/azrtydxb/kuvryn-sync/internal/version.Version=v0.3.0"
//
// and it is "dev" otherwise.
var Version = "dev"
