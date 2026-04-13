package main

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"sync"
	"sync/atomic"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
)

type healthServer struct {
	grpc_health_v1.UnimplementedHealthServer

	server   *grpc.Server
	listener net.Listener
	wg       sync.WaitGroup
	serving  atomic.Bool
}

func startHealthcheck(ctx context.Context, port int) (*healthServer, error) {
	if port < 0 {
		return nil, nil
	}

	addr := net.JoinHostPort("", strconv.Itoa(port))
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen for healthcheck service at %s: %w", addr, err)
	}

	server := grpc.NewServer()
	health := &healthServer{
		server:   server,
		listener: listener,
	}
	grpc_health_v1.RegisterHealthServer(server, health)

	health.wg.Add(1)
	go func() {
		defer health.wg.Done()
		klog.FromContext(ctx).Info("starting healthcheck service", "addr", listener.Addr().String())
		if err := server.Serve(listener); err != nil {
			klog.ErrorS(err, "healthcheck service terminated")
		}
	}()

	return health, nil
}

func (h *healthServer) SetServing(serving bool) {
	h.serving.Store(serving)
}

func (h *healthServer) Stop() {
	if h.server != nil {
		h.server.GracefulStop()
	}
	if h.listener != nil {
		_ = h.listener.Close()
	}
	h.wg.Wait()
}

func (h *healthServer) Check(_ context.Context, request *grpc_health_v1.HealthCheckRequest) (*grpc_health_v1.HealthCheckResponse, error) {
	switch request.GetService() {
	case "", "liveness":
	default:
		return nil, status.Error(codes.NotFound, "unknown service")
	}

	response := &grpc_health_v1.HealthCheckResponse{
		Status: grpc_health_v1.HealthCheckResponse_NOT_SERVING,
	}
	if h.serving.Load() {
		response.Status = grpc_health_v1.HealthCheckResponse_SERVING
	}

	return response, nil
}
