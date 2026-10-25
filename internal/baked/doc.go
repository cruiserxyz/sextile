// Package baked holds pre-baked JSON tables embedded into the system
// at build time via go:embed. Files in this directory are scripture: read-only,
// never edited by hand, regenerated only via the bake pipeline (Phase 2)
// (the protocol used to live in the design notes, removed in Phase 1).
//
// Currently expected: commands.json (astrolog argv templates + parse rules).
package baked
