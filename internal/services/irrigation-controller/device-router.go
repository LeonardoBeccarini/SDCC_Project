package irrigation_controller

import (
	"context"
	"fmt"
	"google.golang.org/grpc/connectivity"
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

		// Crea la connessione (non bloccante)
		conn, err := grpc.NewClient(
			addr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("new client %s (%s): %w", field, addr, err)
		}

		// Avvia il tentativo di connessione
		conn.Connect()

		// attendi Ready entro 5s
		for {
			st := conn.GetState()
			if st == connectivity.Ready {
				break
			}
			// attende un cambio di stato o il timeout del contesto
			if !conn.WaitForStateChange(dctx, st) {
				_ = conn.Close()
				if err := dctx.Err(); err != nil {
					cancel()
					return nil, fmt.Errorf("connect %s (%s): %w", field, addr, err)
				}
				cancel()
				return nil, fmt.Errorf("connect %s (%s): context done", field, addr)
			}
		}

		// successo: registra client e connessione
		dr.mu.Lock()
		dr.conns[field] = conn
		dr.clis[field] = pb.NewDeviceServiceClient(conn)
		dr.mu.Unlock()

		// cleanup della iterazione
		cancel()
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
