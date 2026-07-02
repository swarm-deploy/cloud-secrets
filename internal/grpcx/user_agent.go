package grpcx

import (
	"google.golang.org/grpc"
)

var userAgent string

func SetUserAgent(ua string) {
	userAgent = ua
}

func WithUserAgent() grpc.DialOption {
	return grpc.WithUserAgent(userAgent)
}
