//go:build !wasip1

package plugin

import (
	"syscall"
)

func createPipe() (int, int, error) {
	fds := make([]int, 2)
	err := syscall.Pipe(fds)
	return fds[0], fds[1], err
}

func setNonBlocking(fd int) error {
	return syscall.SetNonblock(fd, true)
}

func closePipe(r, w int) {
	syscall.Close(r)
	syscall.Close(w)
}

func (p *WasmPlugin) readFromPipe(fd int) string {
	buf := make([]byte, 4096)
	var out []byte
	for {
		n, err := syscall.Read(fd, buf)
		if n <= 0 || err != nil {
			break
		}
		out = append(out, buf[:n]...)
	}
	return string(out)
}
