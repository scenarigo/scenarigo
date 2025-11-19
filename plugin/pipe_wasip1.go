//go:build wasip1

package plugin

import (
	"syscall"
)

func createPipe() (int, int, error) {
	fds := make([]int, 2)
	err := syscall.Pipe(fds)
	return fds[0], fds[1], err
}

func setNonBlocking(_ int) error {
	return nil
}

func closePipe(r, w int) {
	syscall.Close(r)
	syscall.Close(w)
}

func (p *WasmPlugin) readFromPipe(_ int) string {
	return ""
}
