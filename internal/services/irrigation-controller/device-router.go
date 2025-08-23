package irrigation_controller

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	pb "github.com/LeonardoBeccarini/sdcc_project/grpc/gen/go/irrigation"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// deviceRouter mantiene una connessione gRPC per ogni field
type deviceRouter struct {
	mu    sync.RWMutex
	conns map[string]*grpc.ClientConn
	clis  map[string]pb.DeviceServiceClient
}

// Verifica a compile-time che *deviceRouter implementi DeviceRouter
var _ DeviceRouter = (*deviceRouter)(nil)

// NewDeviceRouter accetta una stringa tipo "field1=host1:50051,field2=host2:50051"
func NewDeviceRouter(ctx context.Context, mapStr string) (DeviceRouter, error) {
	dr := &deviceRouter{
		conns: make(map[string]*grpc.ClientConn),
		clis:  make(map[string]pb.DeviceServiceClient),
	}

	pairs := strings.Split(mapStr, ",")
	for _, p := range pairs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		kv := strings.SplitN(p, "=", 2)
		if len(kv) != 2 {
			return nil, fmt.Errorf("invalid DEVICE_GRPC_ADDR_MAP entry: %q", p)
		}
		field, addr := strings.TrimSpace(kv[0]), strings.TrimSpace(kv[1])

		dctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		// Dial bloccante *senza* WithBlock (deprecato): usiamo il timeout + ritorno errore
		conn, err := grpc.DialContext(
			dctx,
			addr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithReturnConnectionError(),
		)
		cancel()
		if err != nil {
			return nil, fmt.Errorf("dial %s (%s): %w", field, addr, err)
		}

		dr.mu.Lock()
		dr.conns[field] = conn
		dr.clis[field] = pb.NewDeviceServiceClient(conn)
		dr.mu.Unlock()
	}
	return dr, nil
}

func (d *deviceRouter) Get(field string) (pb.DeviceServiceClient, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	cli, ok := d.clis[field]
	return cli, ok
}

func (d *deviceRouter) Close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, c := range d.conns {
		if c != nil {
			_ = c.Close()
		}
	}
	d.clis = map[string]pb.DeviceServiceClient{}
	d.conns = map[string]*grpc.ClientConn{}
}
