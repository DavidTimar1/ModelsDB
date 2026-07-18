// Package seedcatalog embeds the published objective catalog so a freshly
// downloaded binary can seed an empty database on its FIRST launch without any
// network access. The embedded bytes are the curated.json export (personal data
// and unlisted models already stripped) baked into the binary at build time.
//
// The file internal/seedcatalog/catalog.json is committed and updated by the
// publish step (export the curated catalog from the maintainer's database, write
// it here, commit). A development build may ship the placeholder, in which case
// Available reports false and the app falls back to the setup/remote path.
package seedcatalog

import _ "embed"

//go:embed catalog.json
var Bytes []byte

// Available reports whether a real catalog is embedded (more than an empty
// placeholder), so callers can fall back to the remote/setup seed when it is not.
func Available() bool { return len(Bytes) > 64 }
