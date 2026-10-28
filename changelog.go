package main

import _ "embed"

// changelog is the repository's CHANGELOG.md, built into the app for What's
// new and the changelog in the Updates section. go:embed reaches only files
// in or below a package's folder, so the root package embeds it and main
// hands it on through the config, like the build version.
//
//go:embed CHANGELOG.md
var changelog string
