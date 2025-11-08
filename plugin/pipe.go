//go:build !wasip1

package plugin

import (
	"os"
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

func (p *WasmPlugin) readFromPipe(fd int) string {
	buf := make([]byte, 4096)
	var out []byte
	for {
		n, err := syscall.Read(fd, buf)
		if n > 0 {
			out = append(out, buf[:n]...)
		}
		if err != nil {
			if err == syscall.EAGAIN || err == syscall.EWOULDBLOCK {
				break
			}
			if err == os.ErrClosed {
				break
			}
			return string(out)
		}
		if n == 0 {
			break
		}
	}
	return string(out)
}
