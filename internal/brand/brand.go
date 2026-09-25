// Package brand holds the product's former name so tests can prove it is gone.
package brand

// OldName is what Kuvryn Sync was called up to v0.3.0. Tests that assert the
// old name is absent spell it through this constant, so the repository-wide
// guard in this package can exclude this package alone.
const OldName = "solder"
