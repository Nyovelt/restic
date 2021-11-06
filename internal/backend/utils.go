package backend

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/restic/restic/internal/restic"
)

// LoadAll reads all data stored in the backend for the handle into the given
// buffer, which is truncated. If the buffer is not large enough or nil, a new
// one is allocated.
func LoadAll(ctx context.Context, buf []byte, be restic.Backend, h restic.Handle) ([]byte, error) {
	err := be.Load(ctx, h, 0, 0, func(rd io.Reader) error {
		// make sure this is idempotent, in case an error occurs this function may be called multiple times!
		wr := bytes.NewBuffer(buf[:0])
		_, cerr := io.Copy(wr, rd)
		if cerr != nil {
			return cerr
		}
		buf = wr.Bytes()
		return nil
	})

	if err != nil {
		return nil, err
	}

	return buf, nil
}

// LimitedReadCloser wraps io.LimitedReader and exposes the Close() method.
type LimitedReadCloser struct {
	io.Closer
	io.LimitedReader
}

// LimitReadCloser returns a new reader wraps r in an io.LimitedReader, but also
// exposes the Close() method.
func LimitReadCloser(r io.ReadCloser, n int64) *LimitedReadCloser {
	return &LimitedReadCloser{Closer: r, LimitedReader: io.LimitedReader{R: r, N: n}}
}

// DefaultLoad implements Backend.Load using lower-level openReader func
func DefaultLoad(ctx context.Context, h restic.Handle, length int, offset int64,
	openReader func(ctx context.Context, h restic.Handle, length int, offset int64) (io.ReadCloser, error),
	fn func(rd io.Reader) error) error {
	rd, err := openReader(ctx, h, length, offset)
	if err != nil {
		return err
	}
	err = fn(rd)
	if err != nil {
		_ = rd.Close() // ignore secondary errors closing the reader
		return err
	}
	return rd.Close()
}

type memorizedLister struct {
	fileInfos []restic.FileInfo
	tpe       restic.FileType
}

func (m *memorizedLister) List(ctx context.Context, t restic.FileType, fn func(restic.FileInfo) error) error {
	if t != m.tpe {
		return fmt.Errorf("filetype mismatch, expected %s got %s", m.tpe, t)
	}
	for _, fi := range m.fileInfos {
		if ctx.Err() != nil {
			break
		}
		err := fn(fi)
		if err != nil {
			return err
		}
	}
	return ctx.Err()
}

func MemorizeList(ctx context.Context, be restic.Lister, t restic.FileType) (restic.Lister, error) {
	var fileInfos []restic.FileInfo
	err := be.List(ctx, t, func(fi restic.FileInfo) error {
		fileInfos = append(fileInfos, fi)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &memorizedLister{
		fileInfos: fileInfos,
		tpe:       t,
	}, nil
}
