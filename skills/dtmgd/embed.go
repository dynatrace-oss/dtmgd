// Package dtmgdskill embeds the dtmgd skill content (SKILL.md and the
// references/ it points to) so that the `dtmgd skills install` command can
// ship it as part of the binary.
package dtmgdskill

import "embed"

// Content is the embedded filesystem rooted at skills/dtmgd/: SKILL.md and
// the references/ subtree.
//
//go:embed SKILL.md references
var Content embed.FS
