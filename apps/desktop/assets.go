package main

import "embed"

// Desktop builds generate the shared dashboard into frontend/embed/app before
// compilation. The marker keeps the embed root valid for unit tests.
//
//go:embed all:frontend/embed
var embeddedAssets embed.FS
