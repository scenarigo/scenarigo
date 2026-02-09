//go:build wasip1

package plugin

func Open(path string) (Plugin, error) {
	return nil, nil
}

func CloseAll() error {
	return nil
}
