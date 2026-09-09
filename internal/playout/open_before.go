package playout

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"time"
)

// openBlockBefore bounds source opening and its first read without assigning the
// programme lifetime a context deadline equal to its start boundary.
func openBlockBefore(ctx context.Context, end time.Time, open func(context.Context) (Block, error)) (Block, error) {
	if err := ctx.Err(); err != nil {
		return Block{}, err
	}
	if !time.Now().Before(end) {
		return Block{}, context.DeadlineExceeded
	}
	childCtx, cancel := context.WithCancel(ctx)
	timer := time.AfterFunc(time.Until(end), cancel)
	block, err := open(childCtx)
	if err != nil || block.Content == nil {
		timer.Stop()
		cancel()
		if block.Content != nil {
			_ = block.Content.Close()
		}
		if err == nil {
			err = io.ErrUnexpectedEOF
		}
		if ctx.Err() != nil {
			err = ctx.Err()
		} else if !time.Now().Before(end) {
			err = context.DeadlineExceeded
		}
		return Block{}, err
	}
	content := block.Content
	stopClose := context.AfterFunc(childCtx, func() { _ = content.Close() })
	prefix := make([]byte, 188)
	var n int
	for n == 0 && err == nil && childCtx.Err() == nil {
		n, err = content.Read(prefix)
	}
	timely := timer.Stop() && time.Now().Before(end) && childCtx.Err() == nil
	if !timely || n == 0 || (err != nil && !errors.Is(err, io.EOF)) {
		stopClose()
		cancel()
		_ = content.Close()
		if ctx.Err() != nil {
			err = ctx.Err()
		} else if !timely {
			err = context.DeadlineExceeded
		} else if err == nil {
			err = io.ErrUnexpectedEOF
		}
		return Block{}, err
	}
	block.Content = &openedBlockContent{
		reader: io.MultiReader(bytes.NewReader(prefix[:n]), content), content: content,
		cancel: cancel, stopClose: stopClose,
	}
	return block, nil
}

type openedBlockContent struct {
	reader    io.Reader
	content   io.ReadCloser
	cancel    context.CancelFunc
	stopClose func() bool
	once      sync.Once
	err       error
}

func (c *openedBlockContent) Read(p []byte) (int, error) { return c.reader.Read(p) }
func (c *openedBlockContent) Close() error {
	c.once.Do(func() { c.stopClose(); c.err = c.content.Close(); c.cancel() })
	return c.err
}
