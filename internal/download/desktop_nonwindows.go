//go:build !windows

package download

import "fmt"

func nativeDesktop(string, bool) error {
	return fmt.Errorf("native Windows desktop helper is unavailable")
}
