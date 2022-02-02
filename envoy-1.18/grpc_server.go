package main

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alexflint/go-arg"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"

	v3alpha "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_proc/v3alpha"
	pb "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3alpha"
)

var args struct {
	Port                     string `arg:"-p,env:PORT" default:"8080"`
	MaxConcurrentConnections uint32 `arg:"-c,env:MAX_CONNECTIONS" default:"100"`
}

type server struct{}

func (s *server) Check(_ context.Context, _ *healthpb.HealthCheckRequest) (*healthpb.HealthCheckResponse, error) {
	return &healthpb.HealthCheckResponse{Status: healthpb.HealthCheckResponse_SERVING}, nil
}

func (s *server) Watch(_ *healthpb.HealthCheckRequest, _ healthpb.Health_WatchServer) error {
	return status.Error(codes.Unimplemented, "watch is not implemented")
}

func (s *server) Process(srv pb.ExternalProcessor_ProcessServer) error {
	ctx := srv.Context()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		req, err := srv.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return status.Errorf(codes.Unknown, "cannot receive stream request: %v", err)
		}

		resp := &pb.ProcessingResponse{}
		switch v := req.Request.(type) {
		case *pb.ProcessingRequest_RequestHeaders:
			log.Println("Request Headers:")
			r := req.Request
			h := r.(*pb.ProcessingRequest_RequestHeaders)
			for _, n := range h.RequestHeaders.Headers.Headers {
				log.Printf("\t%s: %s", n.Key, n.Value)
			}
			resp = &pb.ProcessingResponse{
				Response: &pb.ProcessingResponse_RequestHeaders{},
			}
		case *pb.ProcessingRequest_RequestBody:
			if v.RequestBody.EndOfStream {
				resp = &pb.ProcessingResponse{
					Response: &pb.ProcessingResponse_RequestBody{},
				}
				var body interface{}
				if err := json.Unmarshal(v.RequestBody.Body, &body); err != nil {
					resp.ModeOverride = &v3alpha.ProcessingMode{
						ResponseHeaderMode: v3alpha.ProcessingMode_SKIP,
						ResponseBodyMode:   v3alpha.ProcessingMode_NONE,
					}
				} else {
					content, _ := json.MarshalIndent(body, "", "  ")
					resp.ModeOverride = &v3alpha.ProcessingMode{
						ResponseHeaderMode: v3alpha.ProcessingMode_SEND,
						ResponseBodyMode:   v3alpha.ProcessingMode_BUFFERED,
					}

					log.Printf("RequestBody: %s", content)
				}
			}
		case *pb.ProcessingRequest_ResponseHeaders:
			log.Println("Response Headers:")
			r := req.Request
			h := r.(*pb.ProcessingRequest_ResponseHeaders)
			for _, n := range h.ResponseHeaders.Headers.Headers {
				log.Printf("\t%s: %s", n.Key, n.Value)
			}
			resp = &pb.ProcessingResponse{
				Response: &pb.ProcessingResponse_ResponseHeaders{},
			}
		case *pb.ProcessingRequest_ResponseBody:
			if v.ResponseBody.EndOfStream {
				resp = &pb.ProcessingResponse{
					Response: &pb.ProcessingResponse_ResponseBody{},
				}
				var body interface{}
				if err := json.Unmarshal(v.ResponseBody.Body, &body); err == nil {
					content, _ := json.MarshalIndent(body, "", "  ")
					log.Printf("Response body: %s", content)
				}
			}
		default:
			log.Printf("Unknown Request type %v\n", v)
		}
		if err := srv.Send(resp); err != nil {
			log.Printf("send error %v", err)
		}
	}
}

func main() {
	arg.MustParse(&args)

	grpcServer := grpc.NewServer(grpc.MaxConcurrentStreams(args.MaxConcurrentConnections))

	serverInstance := &server{}
	pb.RegisterExternalProcessorServer(grpcServer, serverInstance)
	healthpb.RegisterHealthServer(grpcServer, serverInstance)

	lis, err := net.Listen("tcp", args.Port)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	log.Printf("Starting gRPC server on port %s\n", args.Port)

	gracefulStop := make(chan os.Signal)
	signal.Notify(gracefulStop, syscall.SIGTERM)
	signal.Notify(gracefulStop, syscall.SIGINT)
	go func() {
		sig := <-gracefulStop
		log.Printf("caught sig: %+v\nWait for 1 second to finish processing", sig)
		time.Sleep(1 * time.Second)
		os.Exit(0)
	}()
	_ = grpcServer.Serve(lis)
}
