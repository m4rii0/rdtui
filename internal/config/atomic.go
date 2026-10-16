package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// atomicOps keeps failure injection local to a write, without changing shared state.
type atomicOps struct {
	createTemp func(string, string) (*os.File, error)
	write      func(*os.File, []byte) (int, error)
	sync       func(*os.File) error
	close      func(*os.File) error
	replace    func(string, string) error
	remove     func(string) error
}

var defaultAtomicOps = atomicOps{
	createTemp: os.CreateTemp,
	write:      (*os.File).Write,
	sync:       (*os.File).Sync,
	close:      (*os.File).Close,
	replace:    replaceFile,
	remove:     os.Remove,
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	return atomicWriteWithOps(path, data, mode, defaultAtomicOps)
}

func atomicWriteWithOps(path string, data []byte, mode os.FileMode, ops atomicOps) (err error) {
	f, err := ops.createTemp(filepath.Dir(path), "."+filepath.Base(path)+"-*")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	closed, replaced := false, false
	defer func() {
		if !closed {
			if closeErr := ops.close(f); closeErr != nil {
				err = errors.Join(err, fmt.Errorf("close temporary file: %w", closeErr))
			}
		}
		if !replaced {
			if removeErr := ops.remove(f.Name()); removeErr != nil {
				err = errors.Join(err, fmt.Errorf("remove temporary file: %w", removeErr))
			}
		}
	}()
	if err := f.Chmod(mode); err != nil {
		return fmt.Errorf("set temporary file permissions: %w", err)
	}
	n, err := ops.write(f, data)
	if err != nil {
		return fmt.Errorf("write temporary file: %w", err)
	}
	if n != len(data) {
		return fmt.Errorf("write temporary file: %w", io.ErrShortWrite)
	}
	if err := ops.sync(f); err != nil {
		return fmt.Errorf("sync temporary file: %w", err)
	}
	// Close may return an error after releasing the handle; never retry it.
	closed = true
	if err := ops.close(f); err != nil {
		return fmt.Errorf("close temporary file: %w", err)
	}
	if err := ops.replace(f.Name(), path); err != nil {
		return fmt.Errorf("replace destination: %w", err)
	}
	replaced = true
	return nil
}
