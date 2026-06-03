// Package dtmgdskill embeds the dtmgd skill content (SKILL.md) so that the
// `dtmgd skills install` command can ship it as part of the binary.
package dtmgdskill

import "embed"

// Content is the embedded filesystem rooted at skills/dtmgd/.
//
//go:embed SKILL.md
var Content embed.FS
