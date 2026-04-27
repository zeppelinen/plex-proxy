package tcpforward

import (
	"context"
	"io"
	"log/slog"
	"net"
	"sync"
)

type Forwarder struct {
	ListenAddr string
	TargetAddr string
	Logger     *slog.Logger
}

func (f Forwarder) Serve(ctx context.Context) error {
	ln, err := net.Listen("tcp", f.ListenAddr)
	if err != nil {
		return err
	}
	defer ln.Close()
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		go f.handle(ctx, conn)
	}
}

func (f Forwarder) handle(ctx context.Context, client net.Conn) {
	defer client.Close()
	target, err := (&net.Dialer{}).DialContext(ctx, "tcp", f.TargetAddr)
	if err != nil {
		if f.Logger != nil {
			f.Logger.Warn("tcp forward dial failed", "target", f.TargetAddr, "error", err)
		}
		return
	}
	defer target.Close()
	var wg sync.WaitGroup
	wg.Add(2)
	go copyAndClose(&wg, target, client)
	go copyAndClose(&wg, client, target)
	wg.Wait()
}

func copyAndClose(wg *sync.WaitGroup, dst net.Conn, src net.Conn) {
	defer wg.Done()
	_, _ = io.Copy(dst, src)
	_ = dst.Close()
}
