//go:build !windows

package output

import "os"

// enableVT is a no-op outside Windows: terminals there take escape codes as
// they are.
func enableVT(*os.File) bool { return true }
